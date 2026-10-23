"""Makes the mug eval's photos from the downloaded originals.

Mugs photographed more than once keep their real photos; the others get
two re-shots made from the one: closer, tilted, darker or softer, as a
second visit with the phone would be. Needs Pillow.
    python3 eval/mugs/make_shots.py
"""
import glob
import os

from PIL import Image, ImageEnhance, ImageFilter

HERE = os.path.dirname(os.path.abspath(__file__))
ORIG, OUT = f"{HERE}/images/orig", f"{HERE}/images/mugs"


def reshot(im, kind):
    w, h = im.size
    if kind == 1:
        a = im.crop((int(w * .08), int(h * .06), int(w * .94), int(h * .95))).rotate(6, fillcolor=(120, 120, 120))
        return ImageEnhance.Brightness(a).enhance(0.82)
    b = im.crop((int(w * .02), int(h * .12), int(w * .88), int(h * .98))).rotate(-8, fillcolor=(90, 90, 90))
    return ImageEnhance.Contrast(b).enhance(1.15).filter(ImageFilter.GaussianBlur(1.2))


for d in sorted(glob.glob(f"{ORIG}/*")):
    name = os.path.basename(d)
    os.makedirs(f"{OUT}/{name}", exist_ok=True)
    real = [Image.open(f).convert("RGB") for f in sorted(glob.glob(f"{d}/*.jpg"))]
    if name == "starbucks_calgary":
        # Two mugs in the photo: keep the one in front.
        w, h = real[0].size
        real[0] = real[0].crop((0, 0, int(w * .62), h))
    shots = list(real)
    if len(real) == 1:
        shots += [reshot(real[0], 1), reshot(real[0], 2)]
    elif len(real) == 2:
        shots.append(reshot(real[1], 2))
    for i, im in enumerate(shots):
        im.save(f"{OUT}/{name}/v{i}.jpg", quality=85 if i < len(real) else 65)
