"""Scores telling objects apart by their geometry, after an embedding picks
the candidates.

Rigid things (a socket, a mug, a meter) do not change shape: the same one
photographed twice has the same scratches, print and edges in the same
places relative to each other. For each photo, an embedding model picks the
K most similar other photos; LightGlue then matches keypoints between the
photo and each candidate, and RANSAC keeps the matches one geometric
transform explains. The candidate with the most such inliers wins, and the
inlier count is the confidence. Scored as identify.py scores embeddings.

    python eval/visual/verify.py --labels L.tsv --images DIR [--boxes B.tsv]
"""

import argparse
import json
import sys
import time
from pathlib import Path

import cv2
import numpy as np
import torch
from lightglue import LightGlue, SuperPoint
from lightglue.utils import rbd

sys.path.insert(0, str(Path(__file__).parent))
import identify  # noqa: E402

TARGET = identify.TARGET


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--labels", help="TSV of path and individual; without it, one folder per individual")
    ap.add_argument("--images", required=True)
    ap.add_argument("--boxes")
    ap.add_argument("--model", default="dinov2_b", choices=sorted(identify.MODELS))
    ap.add_argument("--k", type=int, default=5, help="candidates checked per photo")
    ap.add_argument("--edge", type=int, default=768, help="long edge the keypoints are found at")
    ap.add_argument("--name", default="")
    args = ap.parse_args()

    device = identify.default_device()
    rows = identify.load_labels(args.labels, args.images)
    boxes = identify.load_boxes(args.boxes)
    images = [identify.open_image(Path(args.images) / p, boxes.get(p)) for p, _ in rows]
    who = np.array([w for _, w in rows])
    start = time.time()
    emb = identify.Embedder(args.model, device).embed(images)
    sim = emb @ emb.T
    np.fill_diagonal(sim, -1)
    same = who[:, None] == who[None, :]

    extractor = SuperPoint(max_num_keypoints=1024).eval().to(device)
    matcher = LightGlue(features="superpoint").eval().to(device)

    @torch.no_grad()
    def features(im):
        im = im.copy()
        im.thumbnail((args.edge, args.edge))
        t = torch.from_numpy(np.asarray(im.convert("L"), dtype=np.float32) / 255)[None, None].to(device)
        return extractor.extract(t)

    feats = [features(im) for im in images]
    cache = {}

    @torch.no_grad()
    def inliers(i, j):
        key = (min(i, j), max(i, j))
        if key not in cache:
            out = rbd(matcher({"image0": feats[i], "image1": feats[j]}))
            m = out["matches"].cpu().numpy()
            n = 0
            if len(m) >= 8:
                k0 = rbd(feats[i])["keypoints"].cpu().numpy()[m[:, 0]]
                k1 = rbd(feats[j])["keypoints"].cpu().numpy()[m[:, 1]]
                try:
                    _, mask = cv2.findFundamentalMat(k0, k1, cv2.USAC_MAGSAC, 3.0, 0.999, 10000)
                    n = int(mask.sum()) if mask is not None else 0
                except cv2.error:
                    # Matches no transform explains (all on a line): none hold.
                    n = 0
            cache[key] = n
        return cache[key]

    def best(i, allowed):
        cands = [j for j in np.argsort(-sim[i]) if allowed[j] and j != i][: args.k]
        scored = [(inliers(i, j), j) for j in cands]
        return max(scored) if scored else (0, -1)

    has_mate = same.sum(1) > 1
    known, new = [], []
    for i in range(len(rows)):
        if has_mate[i]:
            n, j = best(i, np.ones(len(rows), bool))
            known.append((n, who[j] == who[i]))
        n, _ = best(i, ~same[i])
        new.append(n)
    known_n = np.array([k[0] for k in known])
    known_right = np.array([k[1] for k in known])
    new_n = np.array(new)

    best_t, best_cov = None, 0.0
    for t in np.unique(np.concatenate([known_n, new_n])):
        fk, fn = known_n >= t, new_n >= t
        n = fk.sum() + fn.sum()
        if n and (fk & known_right).sum() / n >= TARGET and fk.mean() > best_cov:
            best_t, best_cov = int(t), float(fk.mean())
    print(json.dumps({
        "set": args.name or Path(args.labels or args.images).stem, "model": f"{args.model}+lightglue@{args.k}", "boxes": bool(boxes),
        "photos": len(rows), "individuals": int(len(set(who))),
        "rank1": round(float(known_right.mean()), 3),
        "threshold_98": best_t, "auto_at_98": round(best_cov, 3),
        "new_wrongly_filed_at_t": None if best_t is None else round(float((new_n >= best_t).mean()), 3),
        "ms_per_photo": round((time.time() - start) / len(rows) * 1000),
    }, ensure_ascii=False))


if __name__ == "__main__":
    sys.exit(main())
