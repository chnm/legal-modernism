# Use Alpine Linux as base image for smaller size
FROM alpine:3.23 AS build-stage

ARG hugobuildargs
ENV HUGO_BUILD_ARGS=$hugobuildargs

# Set Hugo version - update this to the latest version
ENV HUGO_VERSION=0.154.5
ENV HUGO_BINARY=hugo_extended_${HUGO_VERSION}_Linux-64bit.tar.gz

# Install Hugo and dependencies
RUN apk add --no-cache \
    wget \
    ca-certificates \
    gcompat \
    libstdc++ && \
    ARCH=$(uname -m) && \
    case ${ARCH} in \
        x86_64) HUGO_ARCH="Linux-64bit" ;; \
        aarch64) HUGO_ARCH="Linux-ARM64" ;; \
        armv7l) HUGO_ARCH="Linux-ARM" ;; \
        *) echo "Unsupported architecture: ${ARCH}" && exit 1 ;; \
    esac && \
    HUGO_BINARY=hugo_extended_${HUGO_VERSION}_${HUGO_ARCH}.tar.gz && \
    wget https://github.com/gohugoio/hugo/releases/download/v${HUGO_VERSION}/${HUGO_BINARY} && \
    tar xzf ${HUGO_BINARY} && \
    mv hugo /usr/local/bin/hugo && \
    rm ${HUGO_BINARY} && \
    apk del wget

RUN hugo version

# Set working directory
WORKDIR /app

ADD . .

RUN hugo ${HUGO_BUILD_ARGS}

FROM nginx:1.23-alpine

COPY --from=build-stage /app/public/ /usr/share/nginx/html

