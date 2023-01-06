ARG CUDA_IMAGE=cuda
ARG CUDA_VERSION=11.6.1
ARG BASE_DIST=ubuntu20.04

FROM nvidia/${CUDA_IMAGE}:${CUDA_VERSION}-base-${BASE_DIST}

ARG USER=nebuly

# Add non-root user and give it
# permissions for writing to log dir
RUN useradd --create-home --shell /bin/bash ${USER}
WORKDIR  /home/nebuly

# Add entrypoint script
COPY entrypoint.sh .
RUN chmod +x entrypoint.sh

USER 1000
ENTRYPOINT ["./entrypoint"]
