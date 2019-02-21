FROM alpine:3.9
RUN apk add --no-cache ca-certificates
COPY ./blog-store /sbin/blog-store
ENTRYPOINT ["/sbin/blog-store"]