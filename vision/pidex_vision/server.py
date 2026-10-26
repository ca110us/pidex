"""The look service: a photo in, the vector of what its subject looks like out.

    POST /v1/embed   body: the image (JPEG or PNG bytes)
                     query: box=x,y,w,h (optional, normalised, top-left origin)
                     -> {"model": "dinov2_b@336", "dims": 768, "vector": [...]}
    GET  /healthz    -> {"model": ..., "dims": ..., "device": ...}

Configured by environment: LOOK_MODEL (default dinov2_b), LOOK_FLIP=1 to
also embed the mirror image, LOOK_DEVICE (default: mps, cuda or cpu,
whichever is there), LOOK_MAX_BYTES (default 30 MB).
"""

import io
import os
import threading

from fastapi import FastAPI, HTTPException, Query, Request
from PIL import UnidentifiedImageError

from .embed import MODELS, Embedder, default_device, open_image

MODEL = os.environ.get("LOOK_MODEL", "dinov2_b")
if MODEL not in MODELS:
    raise SystemExit(f"LOOK_MODEL must be one of {', '.join(sorted(MODELS))}")
MAX_BYTES = int(os.environ.get("LOOK_MAX_BYTES", str(30 << 20)))

embedder = Embedder(MODEL, os.environ.get("LOOK_DEVICE") or default_device(), os.environ.get("LOOK_FLIP") == "1")
# One image at a time on the model: the GPU and memory stay predictable,
# and requests queue for the few milliseconds each takes.
lock = threading.Lock()

app = FastAPI(title="Pidex look service", docs_url=None, redoc_url=None)


@app.get("/healthz")
def healthz():
    return {"model": embedder.id, "dims": embedder.dims, "device": embedder.device}


@app.post("/v1/embed")
async def embed(request: Request, box: str | None = Query(default=None)):
    data = await request.body()
    if not data or len(data) > MAX_BYTES:
        raise HTTPException(status_code=413 if data else 400, detail=f"the image must have 1 to {MAX_BYTES} bytes")
    parsed = None
    if box:
        try:
            parsed = tuple(float(v) for v in box.split(","))
            if len(parsed) != 4 or not all(0 <= v <= 1 for v in parsed):
                raise ValueError
        except ValueError:
            raise HTTPException(status_code=400, detail="box must be x,y,w,h between 0 and 1")
    try:
        im = open_image(io.BytesIO(data), parsed)
    except (UnidentifiedImageError, OSError):
        raise HTTPException(status_code=400, detail="not an image")
    with lock:
        vec = embedder.embed([im])[0]
    return {"model": embedder.id, "dims": embedder.dims, "vector": [round(float(v), 6) for v in vec]}
