FROM ubuntu:24.04

RUN apt-get update \
    && apt-get install -y --no-install-recommends openssh-sftp-server \
    && rm -rf /var/lib/apt/lists/*
