# Permissions: why no runtime grant?

chromekit exposes `Browser.GrantPermissions(origin, "geo", "notifications")` — Firefox does not. firefoxkit's `GrantPermissions` always returns `ErrPermissionsRuntimeUnsupported`.

## Why

The W3C WebDriver BiDi spec, as of Firefox 130, does **not** define a permission-grant command. Chrome's CDP has `Browser.grantPermissions`; Firefox's BiDi has nothing equivalent. The permission DB (`permissions.sqlite`) can only be written safely while Firefox is **not** running.

## Workarounds

1. **Launch-time blanket allow/block** via `WithPermissionsDefault`:

   ```go
   firefoxkit.Launch(path,
       firefoxkit.WithPermissionsDefault(firefoxkit.PermGeo, true),
       firefoxkit.WithPermissionsDefault(firefoxkit.PermNotifications, false),
   )
   ```

   This writes `permissions.default.geo = 1` (allow) / `2` (block) into `user.js` before Firefox starts.

2. **Raw pref escape hatch** for permissions not covered by the constants:

   ```go
   firefoxkit.WithPref("permissions.default.camera", 1)
   ```

3. **Persistent profile** — manually grant once in a real Firefox session against the profile dir you pass to `WithUserDataDir`. firefoxkit will reuse those grants on subsequent runs.

## Known permission keys

| Constant | Pref key | Notes |
|---|---|---|
| `PermGeo` | `permissions.default.geo` | geolocation |
| `PermNotifications` | `permissions.default.desktop-notification` | desktop notifications |
| `PermCamera` | `permissions.default.camera` | camera |
| `PermMicrophone` | `permissions.default.microphone` | microphone |
| `PermXR` | `permissions.default.xr` | WebXR / VR |

Values: `0 = ask` (default), `1 = allow`, `2 = block`.

## Future

Per-origin runtime grants would require either:
- BiDi spec extension (track [w3c/webdriver-bidi#issues](https://github.com/w3c/webdriver-bidi/issues)), or
- A startup add-on that exposes a JS bridge — adds attack surface and maintenance load.

Neither is on the firefoxkit roadmap.
