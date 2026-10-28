package skills

import (
	"archive/tar"
	"compress/gzip"
	"context"
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
)

const (
	DefaultRepo = "https://github.com/mgerdov-igt/ai-cookbook"
	DefaultRef  = "main"

	maxArchiveBytes = 20 << 20
	remoteTimeout   = 15 * time.Second
)

var (
	refPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,100}$`)
	shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
	repoPart   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)
)

// Remote fetches a public GitHub repository snapshot over HTTPS. It sends
// only GET requests with no credentials.
type Remote struct {
	Owner, Repo, Ref string
	API, Codeload    string
	Client           *http.Client
}

// ParseRepo validates an https://github.com/<owner>/<repo> URL.
func ParseRepo(raw string) (owner, repo string, err error) {
	parsed, err := url.Parse(strings.TrimSuffix(strings.TrimSpace(raw), ".git"))
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", fmt.Errorf("skills.repo must look like https://github.com/<owner>/<repo>, got %q", raw)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 2 || !repoPart.MatchString(parts[0]) || !repoPart.MatchString(parts[1]) {
		return "", "", fmt.Errorf("skills.repo must look like https://github.com/<owner>/<repo>, got %q", raw)
	}
	return parts[0], parts[1], nil
}

// ValidRef reports whether ref is a safe branch, tag or commit name.
func ValidRef(ref string) bool {
	return refPattern.MatchString(ref) && !strings.Contains(ref, "..")
}

// NewRemote builds a Remote for a validated repo URL and ref.
func NewRemote(repoURL, ref string) (*Remote, error) {
	owner, repo, err := ParseRepo(repoURL)
	if err != nil {
		return nil, err
	}
	if ref == "" {
		ref = DefaultRef
	}
	if !ValidRef(ref) {
		return nil, fmt.Errorf("skills.ref %q is not a safe ref name", ref)
	}
	return &Remote{
		Owner: owner, Repo: repo, Ref: ref,
		API: "https://api.github.com", Codeload: "https://codeload.github.com",
		Client: &http.Client{Timeout: remoteTimeout},
	}, nil
}

// Key names the folder that holds this repo's fetched snapshots.
func (r *Remote) Key() string { return r.Owner + "-" + r.Repo }

// Latest returns the commit SHA of the ref. When etag matches the last
// answer, changed is false and sha is empty.
func (r *Remote) Latest(ctx context.Context, etag string) (sha, newETag string, changed bool, err error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/commits/%s", r.API, r.Owner, r.Repo, r.Ref)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", false, err
	}
	request.Header.Set("Accept", "application/vnd.github.sha")
	request.Header.Set("User-Agent", "TauShell")
	if etag != "" {
		request.Header.Set("If-None-Match", etag)
	}
	response, err := r.Client.Do(request)
	if err != nil {
		return "", "", false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified {
		return "", etag, false, nil
	}
	if response.StatusCode != http.StatusOK {
		return "", "", false, fmt.Errorf("GitHub returned %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 256))
	if err != nil {
		return "", "", false, err
	}
	sha = strings.TrimSpace(string(body))
	if !shaPattern.MatchString(sha) {
		return "", "", false, errors.New("GitHub returned an unexpected commit id")
	}
	return sha, response.Header.Get("ETag"), true, nil
}

// Fetch downloads the snapshot of sha and extracts only tau-shell.yaml and
// the skills folder into dest, which is written to a temporary folder and
// renamed so it is complete or absent.
func (r *Remote) Fetch(ctx context.Context, sha, dest string) error {
	if !shaPattern.MatchString(sha) {
		return fmt.Errorf("invalid commit id %q", sha)
	}
	endpoint := fmt.Sprintf("%s/%s/%s/tar.gz/%s", r.Codeload, r.Owner, r.Repo, sha)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "TauShell")
	response, err := r.Client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub returned %s", response.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	temp, err := os.MkdirTemp(filepath.Dir(dest), ".tmp-")
	if err != nil {
		return err
	}
	if err := extract(io.LimitReader(response.Body, maxArchiveBytes+1), temp); err != nil {
		return errors.Join(err, os.RemoveAll(temp))
	}
	if err := os.Rename(temp, dest); err != nil {
		if _, statErr := os.Stat(dest); statErr == nil {
			return os.RemoveAll(temp)
		}
		return errors.Join(err, os.RemoveAll(temp))
	}
	return nil
}

func extract(source io.Reader, dest string) error {
	counted := &countingReader{reader: source}
	gz, err := gzip.NewReader(counted)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer gz.Close()
	archive := tar.NewReader(gz)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}
		if counted.n > maxArchiveBytes {
			return errors.New("archive is too large")
		}
		// GitHub wraps everything in one <repo>-<sha>/ folder.
		name := path.Clean(header.Name)
		slash := strings.Index(name, "/")
		if slash < 0 {
			continue
		}
		rel := name[slash+1:]
		if rel == "." || rel == "" {
			continue
		}
		if path.IsAbs(rel) || strings.HasPrefix(rel, "../") || strings.Contains(rel, "\\") || strings.Contains(rel, ":") {
			return fmt.Errorf("unsafe path %q in archive", header.Name)
		}
		if rel != ManifestName && !strings.HasPrefix(rel, "skills/") {
			continue
		}
		switch header.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
		case tar.TypeXGlobalHeader:
			continue
		default:
			return fmt.Errorf("archive entry %q is a link or special file", header.Name)
		}
		if header.Size > maxSkillFileBytes {
			return fmt.Errorf("archive entry %q is too large", header.Name)
		}
		target := filepath.Join(dest, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(file, io.LimitReader(archive, maxSkillFileBytes))
		if err := errors.Join(copyErr, file.Close()); err != nil {
			return err
		}
	}
	if counted.n > maxArchiveBytes {
		return errors.New("archive is too large")
	}
	return nil
}

type countingReader struct {
	reader io.Reader
	n      int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	c.n += int64(n)
	return n, err
}
