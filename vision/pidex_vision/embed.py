"""What a photo of an individual looks like, as a vector.

Photos of the same cat, dog or mug come out close together; photos of two
look-alikes come out further apart than words could put them. The server
compares these vectors to find the subjects a new photo most looks like;
eval/visual measures how well each model does it.
"""

from dataclasses import dataclass

import numpy as np
import torch
from PIL import Image, ImageOps

# Name: (timm architecture, input size).
MODELS = {
    # General-purpose self-supervised features, strong on fine detail.
    "dinov2_s": ("vit_small_patch14_dinov2.lvd142m", 336),
    "dinov2_b": ("vit_base_patch14_dinov2.lvd142m", 336),
    "dinov2_l": ("vit_large_patch14_dinov2.lvd142m", 336),
    # Trained to re-identify individual animals across many species.
    "mega_t": ("hf-hub:BVRA/MegaDescriptor-T-224", 224),
    "mega_l": ("hf-hub:BVRA/MegaDescriptor-L-384", 384),
}

# Kept around a subject when it is cut out, so its outline is not lost.
MARGIN = 0.15


def open_image(path_or_file, box=None):
    """An image upright, cut to box (x, y, w, h, normalised, top-left) if given."""
    im = ImageOps.exif_transpose(Image.open(path_or_file)).convert("RGB")
    return crop(im, box) if box else im


def crop(im, box):
    x, y, w, h = box
    W, H = im.size
    x0, y0 = max(0, (x - w * MARGIN) * W), max(0, (y - h * MARGIN) * H)
    x1, y1 = min(W, (x + w * (1 + MARGIN)) * W), min(H, (y + h * (1 + MARGIN)) * H)
    if x1 - x0 < 8 or y1 - y0 < 8:
        return im
    return im.crop((x0, y0, x1, y1))


def default_device():
    if torch.backends.mps.is_available():
        return "mps"
    if torch.cuda.is_available():
        return "cuda"
    return "cpu"


@dataclass
class Embedder:
    """One model, loaded once; embeds images to unit vectors."""

    name: str
    device: str
    flip: bool = False

    def __post_init__(self):
        import timm

        arch, self.size = MODELS[self.name]
        kw = {"pretrained": True, "num_classes": 0}
        if "dinov2" in arch:
            kw["img_size"] = self.size
        self.model = timm.create_model(arch, **kw).eval().to(self.device)
        cfg = timm.data.resolve_data_config({}, model=self.model)
        self.fill = tuple(int(255 * m) for m in cfg["mean"])
        self.mean = torch.tensor(cfg["mean"]).view(3, 1, 1)
        self.std = torch.tensor(cfg["std"]).view(3, 1, 1)
        self.dims = int(self.model.num_features)

    @property
    def id(self):
        """What vectors from it are kept under: change it and they are not comparable."""
        return f"{self.name}@{self.size}" + ("+flip" if self.flip else "")

    def _prep(self, im):
        # The whole subject, padded square: a centre crop could cut off the
        # patch that tells two individuals apart.
        side = max(im.size)
        sq = Image.new("RGB", (side, side), self.fill)
        sq.paste(im, ((side - im.width) // 2, (side - im.height) // 2))
        arr = np.asarray(sq.resize((self.size, self.size), Image.BICUBIC), dtype=np.float32) / 255
        return (torch.from_numpy(arr).permute(2, 0, 1) - self.mean) / self.std

    @torch.no_grad()
    def embed(self, images, batch=16):
        out = []
        for i in range(0, len(images), batch):
            x = torch.stack([self._prep(im) for im in images[i : i + batch]]).to(self.device)
            f = self.model(x)
            if self.flip:
                f = f + self.model(torch.flip(x, dims=[3]))
            out.append(torch.nn.functional.normalize(f, dim=1).cpu())
        return torch.cat(out).numpy()
