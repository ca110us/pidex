# The look service on CPU: no GPU is needed, a photo takes a few hundred
# milliseconds and is embedded in the background.
FROM public.ecr.aws/docker/library/python:3.12-slim-bookworm
WORKDIR /app
COPY requirements.txt .
# torch and torchvision from the CPU index together: a torchvision from PyPI
# does not match a CPU torch and fails on import.
RUN pip install --no-cache-dir --index-url https://download.pytorch.org/whl/cpu torch torchvision \
 && pip install --no-cache-dir -r requirements.txt
COPY pidex_vision ./pidex_vision
# Model weights are downloaded on first start into this volume.
ENV HF_HOME=/models LOOK_DEVICE=cpu
VOLUME /models
EXPOSE 8090
CMD ["uvicorn", "pidex_vision.server:app", "--host", "0.0.0.0", "--port", "8090"]
