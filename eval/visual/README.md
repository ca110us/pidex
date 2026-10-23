# Telling individuals apart by image

Offline evaluation of image models for filing by appearance: which cat, which
dog, which socket. It answers whether an image embedding (and, for rigid
things, keypoint matching) can pick the right individual more often and more
cheaply than the current approach, where a vision model compares photos after
candidates are found by words.

## Tools

- `identify.py` embeds every photo with one model and compares each photo with
  all others, as Pidex compares a new photo with those already filed.
- `verify.py` lets an embedding pick the 5 most similar photos, then matches
  keypoints with LightGlue and keeps the matches one geometric transform
  explains (RANSAC). The most inliers win. For rigid things only.

Both report:

| Field | Meaning |
|---|---|
| `rank1` | With other photos of its individual on file, the closest one is right |
| `threshold_98` | The lowest similarity (or inlier count) at which filing without asking is at least 98% right, counting new individuals wrongly filed |
| `auto_at_98` | Share of photos filed without asking at that threshold |
| `new_wrongly_filed_at_t` | Share of photos of an individual not on file that would still be filed to someone |

Every other photo of an individual is available as a reference, which is more
than a collection has at first; early photos will do somewhat worse.

```sh
python3.12 -m venv .venv && .venv/bin/pip install -r eval/visual/requirements.txt
export SSL_CERT_FILE=$(.venv/bin/python -m certifi)   # python.org builds lack CA certificates
.venv/bin/python eval/visual/identify.py --labels eval/appearance/labels.tsv --images eval/appearance/images --model dinov2_l
.venv/bin/python eval/visual/identify.py --images path/to/sockets --model dinov2_b     # one folder per individual
.venv/bin/python eval/visual/verify.py --images path/to/sockets
```

Labels are `path<TAB>individual` (the last column is the individual, so the
appearance and mug label files work as they are); without `--labels`, each
folder is one individual. `--boxes` crops to the subject box the phone found.

## Results (2026-10-23, Apple M4 Pro)

| Set | Photos / individuals | Best model | rank1 | auto at 98% | New ones wrongly filed |
|---|---|---|---|---|---|
| Cats from Commons (two look-alike tuxedo cats) | 57 / 5 | DINOv2-L + flip | 0.982 | 0.632 | 0 |
| A household's cats | 17 / 4 | DINOv2-L | 1.000 | 1.000 | 0 |
| Dog faces (DogFaceNet sample) | 541 / 100 | DINOv2-B + flip | 0.965 | 0.634 | 0.007 |
| Mugs (`eval/mugs`) | 36 / 12 | DINOv2-B | 0.972 | 0.972 | 0 |
| Products (Stanford Online Products sample: mugs, kettles, coffee makers, toasters) | 513 / 100 | DINOv2-L | 0.727 | 0.236 | 0.004 |

Other models on the same sets:

| Set | DINOv2-B | DINOv2-L | MegaDescriptor-T | MegaDescriptor-L | DINOv2-B + LightGlue |
|---|---|---|---|---|---|
| Cats from Commons, rank1 / auto | 0.965 / 0.614 | 0.982 / 0.596 | 0.702 / 0.035 | 0.825 / 0.526 | |
| Household cats | 0.941 / 0.824 | 1.000 / 1.000 | 0.882 / 0.765 | 0.941 / 0.765 | |
| Dog faces | 0.956 / 0.575 | 0.972 / 0.566 | 0.699 / 0.068 | 0.784 / 0.155 | |
| Mugs | 0.972 / 0.972 | 0.972 / 0.972 | 0.972 / 0.972 | 1.000 / 0.972 | 0.944 / 0.944 |
| Products | 0.712 / 0.226 | 0.727 / 0.236 | | 0.717 / 0.074 | 0.749 / 0.138 |

For comparison, the current approach (words to find candidates, `openai/gpt-6-luna` to compare) filed about 20% of the Commons cats without asking, 94% of them right (see the repository README).

What this says:

- **DINOv2 tells pets apart well**, far better than words: about 60% of photos can be filed without asking at 98% accuracy, against about 20% now, in tens of milliseconds and at no model cost. MegaDescriptor, trained mostly on wildlife, does worse on pets.
- **A photo identifies only if it shows what identifies.** The product photos of one item include the label underneath, the inside, the parts laid out and catalogue renders; neither embeddings nor keypoints can join those, and a person could not either. Capture should guide the first photo of a subject towards a standard view (a pet's face, a socket with its surroundings).
- **Keypoint matching** adds a little where views overlap (product rank1 0.71 to 0.75) but its inlier counts separate right from wrong less cleanly than embedding similarity. It needs real photos of rigid things from overlapping views (the socket set) before it can be judged.

Caveats: the sets are small; the Commons photos may hold near-duplicates from one event; 8 of the 12 mugs have one real photo and two generated retakes, which flatters keypoint matching. The household photos are not in the repository.
