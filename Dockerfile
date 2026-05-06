FROM ubuntu:latest
LABEL authors="Lapp"

ENTRYPOINT ["top", "-b"]