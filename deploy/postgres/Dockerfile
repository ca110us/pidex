# Official Postgres via AWS's public mirror of Docker library images, plus pgvector
# from the PGDG apt repository that the official image already configures.
FROM public.ecr.aws/docker/library/postgres:17-bookworm
RUN apt-get update \
 && apt-get install -y --no-install-recommends postgresql-17-pgvector \
 && rm -rf /var/lib/apt/lists/*
