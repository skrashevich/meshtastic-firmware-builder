FROM python:3.12-slim

ARG MKLITTLEFS_TOOL=platformio/tool-mklittlefs

RUN apt-get update \
    && apt-get install -y --no-install-recommends git ca-certificates build-essential \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir platformio \
    && pio pkg install --global --tool "${MKLITTLEFS_TOOL}" --silent \
    && pio pkg list --global --tool "${MKLITTLEFS_TOOL}"

WORKDIR /workspace/repo

ENTRYPOINT ["pio"]
