FROM golang:1.11.5-alpine3.9
COPY ./blog-store /go/bin/blog-store
ENTRYPOINT ["/go/bin/blog-store"]