FROM alpine:edge AS ffmpeg

ADD https://github.com/FFmpeg/FFmpeg.git /usr/src/ffmpeg
WORKDIR /usr/src/ffmpeg
# FFmpeg build deps
RUN apk add --no-cache \
  build-base \
  nasm \
  yasm \
  pkgconfig \
  libwebp-dev \
  libvpx-dev \
  libopusenc-dev
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
  --enable-encoder=libwebp,libvpx_vp9 \
  --enable-muxer=webm,webp,image2 \
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
RUN apk add --no-cache libwebp-tools libvpx
COPY --from=builder /build/webp-watch-go /
COPY --from=ffmpeg /usr/local /usr/local
ENV DB_FILE=/db/webp-watch-go.gob
ENTRYPOINT ["/webp-watch-go", "/input", "/output"]