FROM alpine:3.9
RUN apk add --no-cache ca-certificates
COPY ./blog-storage /sbin/blog-storage
ENTRYPOINT ["/sbin/blog-storage"]