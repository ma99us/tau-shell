# TauShell Phone (Android, sideload)

Companion app for the TauShell phone server (LAN only, off by default). It lists
sessions, shows their state and sends preset or typed prompts through the PC's
guarded `send` path. Experimental; see
[docs/v3/implementation/04-mobile-companion-plan.md](../docs/v3/implementation/04-mobile-companion-plan.md)
and the "Phone server" section of [docs/v2/11-security-model.md](../docs/v2/11-security-model.md).

## Build

Needs JDK 17 and the Android SDK (platform 36, build-tools).

```powershell
$env:JAVA_HOME = 'C:\path\to\jdk-17'
$env:ANDROID_HOME = 'C:\path\to\Android\sdk'
.\gradlew.bat :app:testDebugUnitTest :app:assembleDebug
```

The APK is `app\build\outputs\apk\debug\app-debug.apk`. For a release build, put a
keystore in the git-ignored `keystore.properties` (`storeFile`, `storePassword`,
`keyAlias`, `keyPassword`) and run `:app:assembleRelease`.

## Install and pair

1. Copy the APK to the phone and allow installing from that source. Compare its
   SHA-256 with the one you built (`Get-FileHash app-debug.apk`).
2. On the PC: Home > Phone, turn the phone server on, press Add phone.
3. In the app scan the QR code (or paste the `taushell://pair?...` link), then
   approve the phone's name on the PC.

Notes: the phone and PC must be on the same network and the PC firewall must allow
the port. `mobile.interface` must be the PC address the phone can actually reach: on
a PC with several adapters (Wi-Fi, Ethernet, VPN) a phone on the same Wi-Fi may reach
only one of them (in a live test the Wi-Fi adapter address failed with "No route to
host" while the Ethernet address on the same subnet worked, with a VPN client running).
Test with `adb shell ping <pc address>` before pairing. Android battery optimization
can delay background notifications; exempt the app if alerts arrive late. QR scanning
uses Google Play services; without it, paste the link.
