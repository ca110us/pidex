"""Scores image embedding models at telling individuals apart.

Each photo is compared with every other photo of its set, the way Pidex
would compare a new photo with the photos already filed:

- rank-1: with its individual on file, is the closest photo of the right one?
- open set: with its individual's other photos removed, the closest photo
  is someone else, so filing it would be wrong; a threshold must stop it.
- at the lowest similarity threshold where filing without asking is at
  least 98% right (counting both), how many photos are filed without asking.

Labels are a TSV of `path<TAB>individual`; paths are relative to --images.
Without --labels, each folder in --images is one individual, named by it.
A boxes TSV (`path<TAB>x<TAB>y<TAB>w<TAB>h`, normalised, top-left origin)
crops each photo to its subject first, as the phone does.

    python eval/visual/identify.py --labels L.tsv --images DIR --model dinov2_b
"""

import argparse
import json
import sys
import time
from pathlib import Path

import numpy as np
import torch
from PIL import Image, ImageOps

MODELS = {
    # General-purpose self-supervised features, strong on fine detail.
    "dinov2_s": ("vit_small_patch14_dinov2.lvd142m", 336),
    "dinov2_b": ("vit_base_patch14_dinov2.lvd142m", 336),
    "dinov2_l": ("vit_large_patch14_dinov2.lvd142m", 336),
    # Trained to re-identify individual animals across many species.
    "mega_t": ("hf-hub:BVRA/MegaDescriptor-T-224", 224),
    "mega_l": ("hf-hub:BVRA/MegaDescriptor-L-384", 384),
}

TARGET = 0.98


IMAGE_TYPES = {".jpg", ".jpeg", ".png", ".heic", ".webp"}


def load_labels(path, images=None):
    if not path:
        # One folder per individual.
        root = Path(images)
        return [(str(f.relative_to(root)), f.parent.name) for f in sorted(root.glob("*/*")) if f.suffix.lower() in IMAGE_TYPES]
    rows = []
    for line in Path(path).read_text().splitlines():
        if not line.strip() or line.startswith("#"):
            continue
        parts = line.split("\t")
        rows.append((parts[0], parts[-1]))
    return rows


def load_boxes(path):
    boxes = {}
    if path:
        for line in Path(path).read_text().splitlines():
            p = line.split("\t")
            if len(p) >= 5:
                boxes[p[0]] = tuple(float(v) for v in p[1:5])
    return boxes


def open_image(path, box, margin=0.15):
    im = ImageOps.exif_transpose(Image.open(path)).convert("RGB")
    if box:
        x, y, w, h = box
        W, H = im.size
        x0, y0 = max(0, (x - w * margin) * W), max(0, (y - h * margin) * H)
        x1, y1 = min(W, (x + w * (1 + margin)) * W), min(H, (y + h * (1 + margin)) * H)
        im = im.crop((x0, y0, x1, y1))
    return im


def embedder(name, device):
    import timm

    arch, size = MODELS[name]
    kw = {"pretrained": True, "num_classes": 0}
    if "dinov2" in arch:
        kw["img_size"] = size
    model = timm.create_model(arch, **kw).eval().to(device)
    cfg = timm.data.resolve_data_config({}, model=model)
    mean, std = torch.tensor(cfg["mean"]).view(3, 1, 1), torch.tensor(cfg["std"]).view(3, 1, 1)

    def prep(im):
        # The whole subject, padded square: a centre crop could cut off
        # the patch that tells two individuals apart.
        side = max(im.size)
        sq = Image.new("RGB", (side, side), tuple(int(255 * m) for m in cfg["mean"]))
        sq.paste(im, ((side - im.width) // 2, (side - im.height) // 2))
        t = torch.from_numpy(np.asarray(sq.resize((size, size), Image.BICUBIC), dtype=np.float32) / 255).permute(2, 0, 1)
        return (t - mean) / std

    @torch.no_grad()
    def embed(images, flip):
        out = []
        for i in range(0, len(images), 16):
            batch = torch.stack([prep(im) for im in images[i : i + 16]]).to(device)
            f = model(batch)
            if flip:
                f = f + model(torch.flip(batch, dims=[3]))
            out.append(torch.nn.functional.normalize(f, dim=1).cpu())
        return torch.cat(out).numpy()

    return embed


def score(emb, who):
    """Rank-1, and auto-filing at the threshold that keeps it ≥98% right."""
    sim = emb @ emb.T
    np.fill_diagonal(sim, -1)
    who = np.array(who)
    same = who[:, None] == who[None, :]
    has_mate = same.sum(1) > 1  # another photo of the same one exists
    # Known: the best match among all other photos.
    best = sim.argmax(1)
    known_sim = sim[np.arange(len(who)), best]
    known_right = who[best] == who
    # New: its own other photos removed; any match is wrong.
    new_sim = np.where(same, -1, sim).max(1)
    q = has_mate
    rank1 = known_right[q].mean()
    best_t, best_cov = None, 0.0
    for t in np.unique(np.concatenate([known_sim[q], new_sim])):
        filed_known = (known_sim[q] >= t)
        filed_new = (new_sim >= t)
        n = filed_known.sum() + filed_new.sum()
        if n == 0:
            continue
        right = (filed_known & known_right[q]).sum()
        if right / n >= TARGET:
            cov = filed_known.mean()
            if cov > best_cov:
                best_t, best_cov = float(t), float(cov)
    return {
        "photos": int(len(who)),
        "individuals": int(len(set(who))),
        "rank1": round(float(rank1), 3),
        "threshold_98": None if best_t is None else round(best_t, 3),
        "auto_at_98": round(best_cov, 3),
        "new_wrongly_filed_at_t": None if best_t is None else round(float((new_sim >= best_t).mean()), 3),
    }


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--labels", help="TSV of path and individual; without it, one folder per individual")
    ap.add_argument("--images", required=True)
    ap.add_argument("--boxes")
    ap.add_argument("--model", default="dinov2_b", choices=sorted(MODELS))
    ap.add_argument("--flip", action="store_true", help="also embed the mirror image (a pet facing the other way)")
    ap.add_argument("--name", default="")
    args = ap.parse_args()

    device = "mps" if torch.backends.mps.is_available() else "cpu"
    rows = load_labels(args.labels, args.images)
    boxes = load_boxes(args.boxes)
    images = [open_image(Path(args.images) / p, boxes.get(p)) for p, _ in rows]
    start = time.time()
    emb = embedder(args.model, device)(images, args.flip)
    per = (time.time() - start) / len(images)
    result = {"set": args.name or Path(args.labels or args.images).stem, "model": args.model + ("+flip" if args.flip else ""),
              "boxes": bool(boxes), **score(emb, [w for _, w in rows]), "ms_per_photo": round(per * 1000)}
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    sys.exit(main())
