FROM alpine:3.9
COPY ./blog-store /sbin/blog-store
ENTRYPOINT ["/sbin/blog-store"]