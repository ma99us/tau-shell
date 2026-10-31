//go:build windows

package tools

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/openpgp"
)

const (
	nodeIndexURL        = "https://nodejs.org/dist/index.json"
	gitReleaseAPIURL    = "https://api.github.com/repos/git-for-windows/git/releases/latest"
	maxMetadataBytes    = 16 << 20
	maxNodeZipBytes     = 256 << 20
	maxPortableGitBytes = 256 << 20
	maxExtractedBytes   = 2 << 30
	maxArchiveEntries   = 20000
)

var (
	nodeVersionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)
	gitTagPattern      = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+\.windows\.[0-9]+$`)
	sha256Pattern      = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
)

var nodeReleaseKeyFingerprints = []string{
	"5BE8A3F6C8A5C01D106C0AD820B1A390B168D356",
	"DD792F5973C6DE52C432CBDAC77ABFA00DDBF2B7",
	"CC68F5A3106FF448322E48ED27F5E38D5B0A215F",
	"890C08DB8579162FEE0DF9DB8BEAB4DFCF555EF4",
	"C82FA3AE1CBEDC6BE46B9360C43CEC45C17AB93C",
	"108F52B48DB57BB0CC439B2997B01419BD92F80A",
	"655F3B5C1FB3FA8D1A0CA6BDE4A7D232B936D2FD",
	"A363A499291CBBC940DD62E41F10027AF002F8B0",
}

type nodeRelease struct {
	Version string          `json:"version"`
	Files   []string        `json:"files"`
	LTS     json.RawMessage `json:"lts"`
}

type gitRelease struct {
	TagName    string     `json:"tag_name"`
	Draft      bool       `json:"draft"`
	Prerelease bool       `json:"prerelease"`
	Assets     []gitAsset `json:"assets"`
}

type gitAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

func managedHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Minute,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 8 || request.URL.Scheme != "https" || !trustedDownloadHost(request.URL.Hostname()) {
				return errors.New("download redirected to an untrusted URL")
			}
			return nil
		},
	}
}

func trustedDownloadHost(host string) bool {
	if strings.EqualFold(host, "nodejs.org") || strings.EqualFold(host, "api.github.com") ||
		strings.EqualFold(host, "github.com") || strings.EqualFold(host, "raw.githubusercontent.com") ||
		strings.EqualFold(host, "release-assets.githubusercontent.com") || strings.EqualFold(host, "objects.githubusercontent.com") {
		return true
	}
	return strings.HasSuffix(strings.ToLower(host), ".githubusercontent.com")
}

func getBytes(ctx context.Context, client *http.Client, address string, limit int64) ([]byte, error) {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Scheme != "https" || !trustedDownloadHost(parsed.Hostname()) {
		return nil, fmt.Errorf("refusing untrusted download URL %q", address)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "TauShell")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download metadata: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download metadata returned HTTP %s", response.Status)
	}
	if response.ContentLength > limit {
		return nil, fmt.Errorf("metadata response exceeds %d bytes", limit)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read metadata response: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("metadata response exceeds %d bytes", limit)
	}
	return data, nil
}

func downloadFile(ctx context.Context, client *http.Client, address, destination string, limit int64) (string, error) {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Scheme != "https" || !trustedDownloadHost(parsed.Hostname()) {
		return "", fmt.Errorf("refusing untrusted download URL %q", address)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "TauShell")
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", filepath.Base(destination), err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s returned HTTP %s", filepath.Base(destination), response.Status)
	}
	if response.ContentLength > limit {
		return "", fmt.Errorf("download %s exceeds %d bytes", filepath.Base(destination), limit)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("create download file: %w", err)
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(response.Body, limit+1))
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return "", fmt.Errorf("write download: %w", err)
	}
	if written > limit {
		return "", fmt.Errorf("download %s exceeds %d bytes", filepath.Base(destination), limit)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func parseLatestNodeLTS(data []byte) (nodeRelease, error) {
	var releases []nodeRelease
	if err := json.Unmarshal(data, &releases); err != nil {
		return nodeRelease{}, fmt.Errorf("parse Node release index: %w", err)
	}
	for _, release := range releases {
		if !nodeVersionPattern.MatchString(release.Version) || !hasNodeWinX64Zip(release.Files) || !isLTSValue(release.LTS) {
			continue
		}
		return release, nil
	}
	return nodeRelease{}, errors.New("Node release index has no valid Windows x64 LTS ZIP")
}

func isLTSValue(value json.RawMessage) bool {
	var text string
	if json.Unmarshal(value, &text) == nil {
		return text != ""
	}
	var boolean bool
	if json.Unmarshal(value, &boolean) == nil {
		return boolean
	}
	return false
}

func hasNodeWinX64Zip(files []string) bool {
	for _, file := range files {
		if file == "win-x64-zip" {
			return true
		}
	}
	return false
}

func verifyNodeChecksumSignature(ctx context.Context, client *http.Client, version string, manifest, signature []byte) error {
	var keyring openpgp.EntityList
	for _, fingerprint := range nodeReleaseKeyFingerprints {
		address := "https://raw.githubusercontent.com/nodejs/release-keys/main/keys/" + fingerprint + ".asc"
		armored, err := getBytes(ctx, client, address, 1<<20)
		if err != nil {
			continue
		}
		entities, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(armored))
		if err != nil {
			continue
		}
		for _, entity := range entities {
			got := strings.ToUpper(hex.EncodeToString(entity.PrimaryKey.Fingerprint[:]))
			if got == fingerprint {
				keyring = append(keyring, entity)
			}
		}
	}
	if len(keyring) == 0 {
		return errors.New("could not retrieve any pinned Node.js release signing keys")
	}
	signer, err := openpgp.CheckDetachedSignature(keyring, bytes.NewReader(manifest), bytes.NewReader(signature))
	if err != nil {
		return fmt.Errorf("verify Node.js checksum signature: %w", err)
	}
	got := strings.ToUpper(hex.EncodeToString(signer.PrimaryKey.Fingerprint[:]))
	for _, fingerprint := range nodeReleaseKeyFingerprints {
		if got == fingerprint {
			return nil
		}
	}
	return errors.New("Node.js checksum was signed by an unauthorized key")
}

func parseNodeChecksum(manifest []byte, filename string) (string, error) {
	for _, line := range strings.Split(string(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || !sha256Pattern.MatchString(fields[0]) {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") == filename {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("Node checksum manifest has no valid entry for %s", filename)
}

func parseLatestPortableGit(data []byte) (gitRelease, gitAsset, string, error) {
	var release gitRelease
	if err := json.Unmarshal(data, &release); err != nil {
		return gitRelease{}, gitAsset{}, "", fmt.Errorf("parse Git for Windows release: %w", err)
	}
	if release.Draft || release.Prerelease || !gitTagPattern.MatchString(release.TagName) {
		return gitRelease{}, gitAsset{}, "", errors.New("GitHub returned an invalid stable Git for Windows release")
	}
	for _, asset := range release.Assets {
		if !strings.HasPrefix(asset.Name, "Git-") || !strings.HasSuffix(asset.Name, "-64-bit.tar.bz2") ||
			!sha256Pattern.MatchString(strings.TrimPrefix(asset.Digest, "sha256:")) || !strings.HasPrefix(asset.Digest, "sha256:") {
			continue
		}
		wantURL := "https://github.com/git-for-windows/git/releases/download/" + url.PathEscape(release.TagName) + "/" + url.PathEscape(asset.Name)
		if asset.BrowserDownloadURL != wantURL {
			continue
		}
		return release, asset, strings.TrimPrefix(strings.ToLower(asset.Digest), "sha256:"), nil
	}
	return gitRelease{}, gitAsset{}, "", errors.New("stable Git for Windows release has no verified full x64 PortableGit archive")
}

func InstallManagedTool(ctx context.Context, layout ManagedToolLayout, spec Spec) (string, error) {
	client := managedHTTPClient()
	switch spec.Install.ManagedStrategy {
	case ManagedNodeLTS:
		return installNodeLTS(ctx, client, layout)
	case ManagedPortableGit:
		return installPortableGit(ctx, client, layout)
	default:
		return "", fmt.Errorf("tool %s has no managed install strategy", spec.ID)
	}
}

func installNodeLTS(ctx context.Context, client *http.Client, layout ManagedToolLayout) (string, error) {
	index, err := getBytes(ctx, client, nodeIndexURL, maxMetadataBytes)
	if err != nil {
		return "", err
	}
	release, err := parseLatestNodeLTS(index)
	if err != nil {
		return "", err
	}
	version := strings.TrimPrefix(release.Version, "v")
	filename := "node-" + release.Version + "-win-x64.zip"
	archiveURL := "https://nodejs.org/dist/" + release.Version + "/" + filename
	manifestURL := "https://nodejs.org/dist/" + release.Version + "/SHASUMS256.txt"
	manifest, err := getBytes(ctx, client, manifestURL, maxMetadataBytes)
	if err != nil {
		return "", err
	}
	signature, err := getBytes(ctx, client, manifestURL+".sig", 1<<20)
	if err != nil {
		return "", err
	}
	if err := verifyNodeChecksumSignature(ctx, client, release.Version, manifest, signature); err != nil {
		return "", err
	}
	expectedHash, err := parseNodeChecksum(manifest, filename)
	if err != nil {
		return "", err
	}
	if active, err := layout.ActiveVersion(NodeJS); err == nil && active == version {
		if isDir(layout.NpmPrefix()) && verifyNodeInstall(ctx, layout.NodeVersionDir(version), release.Version) == nil {
			return fmt.Sprintf("Node.js LTS %s is already active.", version), nil
		}
	}
	if err := os.MkdirAll(layout.NodeRoot(), 0o700); err != nil {
		return "", fmt.Errorf("create managed Node directory: %w", err)
	}
	final := layout.NodeVersionDir(version)
	if _, err := os.Stat(final); err == nil {
		if exists(filepath.Join(final, ".taushell-installing")) {
			if err := os.RemoveAll(final); err != nil {
				return "", fmt.Errorf("remove incomplete managed Node install: %w", err)
			}
		} else if exists(filepath.Join(final, "node.exe")) {
			if err := verifyNodeInstall(ctx, final, release.Version); err != nil {
				return "", fmt.Errorf("existing managed Node directory is invalid: %w", err)
			}
			return activateManagedVersion(layout, NodeJS, version, fmt.Sprintf("Verified existing Node.js LTS %s with npm.", version))
		} else {
			return "", fmt.Errorf("managed Node directory for version %s already exists but is incomplete", version)
		}
	}
	if err := os.Mkdir(final, 0o700); err != nil {
		return "", fmt.Errorf("create managed Node version directory: %w", err)
	}
	installComplete := false
	defer func() {
		if !installComplete {
			_ = os.RemoveAll(final)
		}
	}()
	if err := os.WriteFile(filepath.Join(final, ".taushell-installing"), []byte("incomplete\n"), 0o600); err != nil {
		return "", fmt.Errorf("write Node install marker: %w", err)
	}
	work, err := os.MkdirTemp(layout.NodeRoot(), ".install-*")
	if err != nil {
		return "", fmt.Errorf("create Node staging directory: %w", err)
	}
	defer os.RemoveAll(work)
	archivePath := filepath.Join(work, filename)
	actualHash, err := downloadFile(ctx, client, archiveURL, archivePath, maxNodeZipBytes)
	if err != nil {
		return "", err
	}
	if actualHash != expectedHash {
		return "", errors.New("Node.js ZIP SHA-256 does not match the signed release manifest")
	}
	if err := extractNodeZip(archivePath, release.Version, final); err != nil {
		return "", err
	}
	if err := verifyNodeInstall(ctx, final, release.Version); err != nil {
		return "", err
	}
	if err := os.MkdirAll(layout.NpmPrefix(), 0o700); err != nil {
		return "", fmt.Errorf("create managed npm prefix: %w", err)
	}
	if err := os.Remove(filepath.Join(final, ".taushell-installing")); err != nil {
		return "", fmt.Errorf("complete Node install marker: %w", err)
	}
	if _, err := activateManagedVersion(layout, NodeJS, version, fmt.Sprintf("Verified and installed Node.js LTS %s with npm.", version)); err != nil {
		return "", err
	}
	installComplete = true
	return fmt.Sprintf("Verified and installed Node.js LTS %s with npm.", version), nil
}

func verifyNodeInstall(ctx context.Context, directory, expectedVersion string) error {
	nodePath := filepath.Join(directory, "node.exe")
	npmPath := filepath.Join(directory, "npm.cmd")
	if !exists(nodePath) || !exists(npmPath) {
		return errors.New("Node.js ZIP is missing node.exe or npm.cmd")
	}
	environment := DefaultEnvironment()
	nodeVersion, err := runProbe(ctx, environment, nodePath, []string{"--version"})
	if err != nil || strings.TrimSpace(string(nodeVersion)) != expectedVersion {
		return fmt.Errorf("verify node.exe version: got %q, err %v", strings.TrimSpace(string(nodeVersion)), err)
	}
	npmVersion, err := runProbe(ctx, environment, npmPath, []string{"--version"})
	if err != nil {
		return fmt.Errorf("verify npm.cmd version: %w", err)
	}
	if _, ok := ParseVersion(string(npmVersion)); !ok {
		return fmt.Errorf("verify npm.cmd version: unrecognized output %q", strings.TrimSpace(string(npmVersion)))
	}
	return nil
}

func installPortableGit(ctx context.Context, client *http.Client, layout ManagedToolLayout) (string, error) {
	metadata, err := getBytes(ctx, client, gitReleaseAPIURL, maxMetadataBytes)
	if err != nil {
		return "", err
	}
	release, asset, expectedHash, err := parseLatestPortableGit(metadata)
	if err != nil {
		return "", err
	}
	version := strings.TrimPrefix(release.TagName, "v")
	if active, err := layout.ActiveVersion(Git); err == nil && active == version {
		if verifyPortableGit(ctx, filepath.Join(layout.GitVersionDir(version), "cmd", "git.exe")) == nil {
			return fmt.Sprintf("Git for Windows %s is already active.", version), nil
		}
	}
	if err := os.MkdirAll(layout.GitRoot(), 0o700); err != nil {
		return "", fmt.Errorf("create managed Git directory: %w", err)
	}
	final := layout.GitVersionDir(version)
	gitPath := filepath.Join(final, "cmd", "git.exe")
	if _, err := os.Stat(final); err == nil {
		if exists(filepath.Join(final, ".taushell-installing")) {
			if err := os.RemoveAll(final); err != nil {
				return "", fmt.Errorf("remove incomplete PortableGit install: %w", err)
			}
		} else if exists(gitPath) {
			if err := verifyPortableGit(ctx, gitPath); err != nil {
				return "", fmt.Errorf("existing managed Git directory is invalid: %w", err)
			}
			return activateManagedVersion(layout, Git, version, fmt.Sprintf("Verified existing Git for Windows %s.", version))
		} else {
			return "", fmt.Errorf("managed Git directory for version %s already exists but is incomplete", version)
		}
	}
	if err := os.Mkdir(final, 0o700); err != nil {
		return "", fmt.Errorf("create managed Git version directory: %w", err)
	}
	installComplete := false
	defer func() {
		if !installComplete {
			_ = os.RemoveAll(final)
		}
	}()
	if err := os.WriteFile(filepath.Join(final, ".taushell-installing"), []byte("incomplete\n"), 0o600); err != nil {
		return "", fmt.Errorf("write PortableGit install marker: %w", err)
	}
	work, err := os.MkdirTemp(layout.GitRoot(), ".install-*")
	if err != nil {
		return "", fmt.Errorf("create Git staging directory: %w", err)
	}
	defer os.RemoveAll(work)
	archivePath := filepath.Join(work, asset.Name)
	actualHash, err := downloadFile(ctx, client, asset.BrowserDownloadURL, archivePath, maxPortableGitBytes)
	if err != nil {
		return "", err
	}
	if actualHash != expectedHash {
		return "", errors.New("PortableGit SHA-256 does not match GitHub release metadata")
	}
	archive, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	extractErr := extractPortableGitTar(bzip2.NewReader(archive), final)
	closeErr := archive.Close()
	if err := errors.Join(extractErr, closeErr); err != nil {
		return "", fmt.Errorf("extract PortableGit archive: %w", err)
	}
	if !exists(gitPath) {
		return "", errors.New("PortableGit archive is missing cmd/git.exe")
	}
	if err := verifyPortableGit(ctx, gitPath); err != nil {
		return "", fmt.Errorf("verify PortableGit executable: %w", err)
	}
	if err := os.Remove(filepath.Join(final, ".taushell-installing")); err != nil {
		return "", fmt.Errorf("complete PortableGit install marker: %w", err)
	}
	if _, err := activateManagedVersion(layout, Git, version, fmt.Sprintf("Verified and installed Git for Windows %s.", version)); err != nil {
		return "", err
	}
	installComplete = true
	return fmt.Sprintf("Verified and installed Git for Windows %s.", version), nil
}

func verifyPortableGit(ctx context.Context, gitPath string) error {
	output, err := runProbe(ctx, DefaultEnvironment(), gitPath, []string{"--version"})
	if err != nil || !strings.Contains(strings.ToLower(string(output)), "git version") {
		return fmt.Errorf("unexpected version output %q, error %v", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func activateManagedVersion(layout ManagedToolLayout, id ID, version, message string) (string, error) {
	previous, previousErr := layout.ActiveVersion(id)
	if err := layout.SetActiveVersion(id, version); err != nil {
		return "", err
	}
	if err := layout.PrependToProcessPath(); err != nil {
		if previousErr == nil {
			_ = layout.SetActiveVersion(id, previous)
		} else {
			_ = layout.clearActiveVersion(id)
		}
		return "", fmt.Errorf("add managed tool to process PATH: %w", err)
	}
	return message, nil
}

func extractNodeZip(filename, version, destination string) error {
	archive, err := zip.OpenReader(filename)
	if err != nil {
		return fmt.Errorf("open Node.js ZIP: %w", err)
	}
	defer archive.Close()
	root := "node-" + version + "-win-x64"
	var total uint64
	entries := 0
	for _, member := range archive.File {
		entries++
		if entries > maxArchiveEntries {
			return errors.New("Node.js ZIP has too many entries")
		}
		if member.UncompressedSize64 > maxExtractedBytes-total {
			return errors.New("Node.js ZIP exceeds the extraction size limit")
		}
		total += member.UncompressedSize64
		name, err := safeArchivePath(member.Name)
		if err != nil {
			return err
		}
		if name == root {
			continue
		}
		prefix := root + "/"
		if !strings.HasPrefix(name, prefix) {
			return fmt.Errorf("Node.js ZIP contains unexpected root entry %q", member.Name)
		}
		relative := strings.TrimPrefix(name, prefix)
		if relative == "" {
			continue
		}
		target := filepath.Join(destination, filepath.FromSlash(relative))
		if !pathWithin(destination, target) {
			return fmt.Errorf("Node.js ZIP path escapes extraction directory: %q", member.Name)
		}
		if member.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("Node.js ZIP contains an unsupported symbolic link %q", member.Name)
		}
		if member.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		input, err := member.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			input.Close()
			return err
		}
		written, copyErr := io.CopyN(output, input, int64(member.UncompressedSize64))
		closeInputErr := input.Close()
		closeOutputErr := output.Close()
		if err := errors.Join(copyErr, closeInputErr, closeOutputErr); err != nil {
			return err
		}
		if uint64(written) != member.UncompressedSize64 {
			return fmt.Errorf("short extraction for %q", member.Name)
		}
	}
	return nil
}

func extractPortableGitTar(reader io.Reader, destination string) error {
	type linkEntry struct{ name, target string }
	var links []linkEntry
	total := int64(0)
	entries := 0
	archive := tar.NewReader(reader)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		entries++
		if entries > maxArchiveEntries {
			return errors.New("PortableGit archive has too many entries")
		}
		name, err := safeArchivePath(header.Name)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, filepath.FromSlash(name))
		if !pathWithin(destination, target) {
			return fmt.Errorf("PortableGit archive path escapes extraction directory: %q", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > maxExtractedBytes-total {
				return errors.New("PortableGit archive exceeds the extraction size limit")
			}
			total += header.Size
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return err
			}
			output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				return err
			}
			written, copyErr := io.CopyN(output, archive, header.Size)
			closeErr := output.Close()
			if err := errors.Join(copyErr, closeErr); err != nil {
				return err
			}
			if written != header.Size {
				return fmt.Errorf("short extraction for %q", header.Name)
			}
		case tar.TypeLink:
			linkTarget, err := safeArchivePath(header.Linkname)
			if err != nil {
				return fmt.Errorf("invalid PortableGit hard link %q: %w", header.Name, err)
			}
			links = append(links, linkEntry{name: name, target: linkTarget})
		case tar.TypeSymlink:
			if !portableGitVirtualLink(name, header.Linkname) {
				return fmt.Errorf("PortableGit archive contains unsupported symbolic link %q", header.Name)
			}
		default:
			return fmt.Errorf("PortableGit archive contains unsupported entry type %d at %q", header.Typeflag, header.Name)
		}
	}
	for _, link := range links {
		destinationPath := filepath.Join(destination, filepath.FromSlash(link.name))
		targetPath := filepath.Join(destination, filepath.FromSlash(link.target))
		if !pathWithin(destination, destinationPath) || !pathWithin(destination, targetPath) {
			return fmt.Errorf("PortableGit hard link escapes extraction directory: %q", link.name)
		}
		if err := os.MkdirAll(filepath.Dir(destinationPath), 0o700); err != nil {
			return err
		}
		if err := os.Link(targetPath, destinationPath); err != nil {
			remaining := maxExtractedBytes - total
			copied, err := copyRegularFile(targetPath, destinationPath, remaining)
			if err != nil {
				return fmt.Errorf("create PortableGit hard link %q: %w", link.name, err)
			}
			total += copied
		}
	}
	return nil
}

func portableGitVirtualLink(name, target string) bool {
	return (strings.HasPrefix(name, "dev/") || name == "etc/mtab") && strings.HasPrefix(target, "/proc/")
}

func copyRegularFile(source, destination string, limit int64) (int64, error) {
	input, err := os.Open(source)
	if err != nil {
		return 0, err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	written, copyErr := io.CopyN(output, input, limit+1)
	closeErr := output.Close()
	if copyErr != nil && !errors.Is(copyErr, io.EOF) {
		_ = os.Remove(destination)
		return 0, errors.Join(copyErr, closeErr)
	}
	if written > limit {
		_ = os.Remove(destination)
		return 0, errors.Join(errors.New("PortableGit hard links exceed the extraction size limit"), closeErr)
	}
	return written, closeErr
}

func safeArchivePath(name string) (string, error) {
	name = strings.ReplaceAll(name, `\`, "/")
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, ":") {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == ".." {
			return "", fmt.Errorf("unsafe archive path %q", name)
		}
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	return clean, nil
}

func pathWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}
