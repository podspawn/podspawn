FROM ubuntu:24.04

RUN apt-get update \
    && apt-get install -y --no-install-recommends openssh-server \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir /run/sshd

COPY podspawn /usr/local/bin/podspawn
RUN chmod +x /usr/local/bin/podspawn

COPY config.yaml /etc/podspawn/config.yaml

RUN mkdir -p /etc/podspawn/keys \
    && mkdir -p /var/lib/podspawn/locks \
    && chmod 1777 /var/lib/podspawn \
    && chmod 1777 /var/lib/podspawn/locks \
    && touch /var/log/podspawn.log \
    && chmod 666 /var/log/podspawn.log


EXPOSE 22
# Test-only: chmod socket for non-root Docker access, set umask so SQLite WAL
# files are world-writable when created by different users
CMD chmod 666 /var/run/docker.sock && umask 0000 && exec /usr/sbin/sshd -D -e
