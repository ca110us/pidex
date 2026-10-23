# Pidex brand

Pidex: Pi (picture) + dex (index). "Snap it. It files itself."

## Icon

`icon/` holds the originals of direction A, "Index P": a P whose top is cut
open like an index tab. They are plain vector SVGs (a background rect and
groups of paths), drawn by hand, with no embedded bitmaps.

| File | Used as |
|---|---|
| `icon/pidex-dark.svg` | The app icon in light and dark mode; the icon and mark inside the app |
| `icon/pidex-light.svg` | Its colours give the launch screen's ink in light mode |
| `icon/pidex-tinted.svg` | Reference for the tinted look; iOS tints a grayscale version itself |

Everything the app uses is rendered from these by one script, from the
repository root:

```sh
swift design/brand/render.swift
```

It writes into `ios/Pidex/Resources/Assets.xcassets`:

- `AppIcon`: 1024×1024 opaque PNGs, default, dark and tinted (grayscale).
- `PidexIcon`: the icon as a vector PDF; the app rounds its corners
  (`PidexIcon` view).
- `PidexMark`: the P alone, its cut transparent, as a template that takes
  any colour (`PidexMark` view).
- `LaunchMark`: the P on the launch screen, ink in light mode and paper in
  dark.

Edit the SVGs, run the script, and commit both. Do not edit the PNGs or PDFs
by hand.

## Explorations

`explorations/` keeps the four directions the icon was chosen from (A was
chosen), with the overview sheet and the designer's notes.

## Colour

Black and white, after Grok; no blue anywhere. `ios/Pidex/App/Theme.swift`
holds every colour, each with a light and a dark value:

- Ink (black in light mode, white in dark) is the accent: primary buttons,
  the camera button, selections, progress. The asset catalog's
  `AccentColor` is ink too, so system controls follow.
- Paper is the page (white, or pure black); surface and raised are flat
  greys one and two steps off it, for cards, bubbles and fields.
- Colour is kept for state alone: amber when something needs a person, red
  when something failed, green when a shot is identified.

The camera is always dark, whatever the phone's appearance.
