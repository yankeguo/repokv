FROM debian:13-slim

RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install -y ca-certificates curl tini && \
    rm -rf /var/lib/apt/lists/*

ENTRYPOINT [ "/usr/bin/tini", "--" ]

WORKDIR /app

ADD repokv repokv

CMD ["/app/repokv"]
