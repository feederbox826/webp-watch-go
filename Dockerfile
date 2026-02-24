FROM alpine:edge AS svt-vp9
ADD https://github.com/OpenVisualCloud/SVT-VP9.git /usr/src/svt-vp9
WORKDIR /usr/src/svt-vp9
RUN apk add --no-cache \
  build-base \
  cmake samurai nasm
RUN cmake -B build -G Ninja \
  -DCMAKE_INSTALL_PREFIX=/usr/local \
  -DCMAKE_INSTALL_LIBDIR=lib \
  -DBUILD_SHARED_LIBS=True \
  -DCMAKE_BUILD_TYPE=Release \
  -DENABLE_ASM=ON \
  -DENABLE_X86_ASM=OFF
RUN cmake --build build
RUN cmake --install build

FROM alpine:edge AS ffmpeg

ADD https://github.com/FFmpeg/FFmpeg.git /usr/src/ffmpeg
ADD https://raw.githubusercontent.com/OpenVisualCloud/SVT-VP9/refs/heads/master/ffmpeg_plugin/master-0001-Add-ability-for-ffmpeg-to-run-svt-vp9.patch /usr/src/ffmpeg/
WORKDIR /usr/src/ffmpeg
# FFmpeg build deps
RUN apk add --no-cache \
  build-base \
  git \
  nasm \
  yasm \
  pkgconfig \
  libwebp-dev \
  libvpx-dev \
  libopusenc-dev
# copy in svt-vp9
COPY --from=svt-vp9 /usr/local /usr/local
# apply patch
RUN git apply master-0001-Add-ability-for-ffmpeg-to-run-svt-vp9.patch
# set CFLAGS (march via runner)
ARG CFLAGS="-O2 -march=native -mtune=native"
RUN ./configure \
  --prefix=/usr/local \
  --disable-debug \
  --disable-doc \
  --disable-ffprobe \
  --disable-ffplay \
  --disable-everything \
  --disable-static \
  --enable-shared \
  # WebM support (video + audio)
  --enable-libvpx \
  --enable-decoder=vp8,vp9,opus \
  --enable-demuxer=webm,matroska \
  --enable-bsf=vp9_metadata,vp9_superframe,vp9_superframe_split,vp9_raw_reorder \
  # WebP support
  --enable-libwebp \
  --enable-encoder=libwebp,libsvt_vp9 \
  --enable-muxer=webm,webp,image2 \
  # libsvt-vp9
  --enable-libsvtvp9 \
  # reading/writing files
  --enable-protocol=file \
  # compilation runtime options
  --enable-lto=auto \
  --enable-small \
  && make -j$(nproc) \
  && make install
RUN strip /usr/local/bin/ffmpeg
RUN strip --strip-unneeded /usr/local/lib/*.so*
RUN \
  echo "**** file cleanup ****" && \
  rm -r \
    /usr/local/lib/pkgconfig \
    /usr/local/share \
    /usr/local/include

FROM golang:1.24-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# strip and trim
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -trimpath -a -o webp-watch-go .

FROM alpine:edge AS final
RUN apk add --no-cache libwebp-tools
COPY --from=builder /build/webp-watch-go /
COPY --from=ffmpeg /usr/local /usr/local
COPY --from=svt-vp9 /usr/local/lib /usr/local/lib
ENV DB_FILE=/db/webp-watch-go.gob
ENTRYPOINT ["/webp-watch-go", "/input", "/output"]