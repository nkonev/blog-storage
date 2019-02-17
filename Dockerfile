FROM golang:1.11.5-alpine3.9
COPY ./blog-store /sbin/blog-store
ENTRYPOINT ["/sbin/blog-store"]