FROM python:3.12-slim

ARG MKLITTLEFS_TOOL=platformio/tool-mklittlefs

RUN apt-get update \
    && apt-get install -y --no-install-recommends git ca-certificates build-essential ccache \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir platformio \
    && pio pkg install --global --tool "${MKLITTLEFS_TOOL}" --silent \
    && pio pkg list --global --tool "${MKLITTLEFS_TOOL}" \
    && install -m 0755 /root/.platformio/packages/tool-mklittlefs/mklittlefs /usr/local/bin/mklittlefs

RUN set -eux; \
    for prefix in xtensa-esp32-elf xtensa-esp-elf riscv32-esp-elf xtensa-lx106-elf arm-none-eabi; do \
      for tool in gcc g++ cc c++ cpp gcc-ar gcc-ranlib gcc-nm; do \
        ln -sf /usr/bin/ccache "/usr/local/bin/${prefix}-${tool}"; \
      done; \
    done

ENV CCACHE_DIR=/root/.platformio/.cache/ccache \
    CCACHE_MAXSIZE=2G \
    CCACHE_COMPRESS=true \
    CCACHE_COMPILERCHECK=content \
    CCACHE_NOHASHDIR=true \
    CCACHE_SLOPPINESS=time_macros \
    PLATFORMIO_BUILD_CACHE_DIR=/root/.platformio/build-cache

WORKDIR /workspace/repo

ENTRYPOINT ["pio"]
