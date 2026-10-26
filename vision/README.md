# Look service

Turns a photo of a subject into a vector of what it looks like, for telling
individuals apart: which cat, which dog, which socket. The server sends each
photo of a subject told apart by appearance (cut to the subject), keeps the
vector, and compares it with the photos already filed (see "Identification by
appearance" in the repository README). `eval/visual/` measures the models with
this same code.

```sh
make up                              # Postgres and this service in Docker (CPU, port 8090, weights cached in a volume)
# or, on a Mac with Apple silicon, faster on its GPU:
python3.12 -m venv vision/.venv && vision/.venv/bin/pip install -r vision/requirements.txt
make vision
```

`make serve` points the server at `http://127.0.0.1:8090` (`LOOK_SERVICE_URL`).
Without the service, or when it does not answer, filing by appearance works
as before (words to find candidates, a vision model to compare).

| Variable | Default | Meaning |
|---|---|---|
| `LOOK_MODEL` | `dinov2_b` | `dinov2_s`, `dinov2_b`, `dinov2_l`, `mega_t`, `mega_l` |
| `LOOK_FLIP` | off | `1` also embeds the mirror image |
| `LOOK_DEVICE` | `mps`, `cuda` or `cpu` | where the model runs |
| `LOOK_MAX_BYTES` | 30 MB | largest image accepted |

Vectors are kept with the model that made them (`dinov2_b@336`); after
changing `LOOK_MODEL`, run `pidex embed-looks -all` so old photos are compared
again.

API: `POST /v1/embed` with the image as the body (optional `?box=x,y,w,h`,
normalised) returns `{"model", "dims", "vector"}`; `GET /healthz` returns the
model and device.
