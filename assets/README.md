# Artwork

These icons are original geometric placeholders created for FediShare.

They are not copied from another project. Replace them with final branding by
keeping the same filenames:

| File | Use |
| --- | --- |
| `icon.png` | Application / favicon source |
| `icon.ico` | Windows application icon |
| `icon.icns` | macOS bundle icon (create with `iconutil` when packaging) |
| `tray-starting.png` | Tray: starting / connecting / indexing |
| `tray-online.png` | Tray: online |
| `tray-offline.png` | Tray: offline |
| `tray-error.png` | Tray: error |
| `tray-paused.png` | Tray: sharing paused |

Regenerate the placeholders with:

```bash
go run ./scripts/genicons
```

`icon.icns` is not generated here. On macOS, packaging can build it from `icon.png`.
