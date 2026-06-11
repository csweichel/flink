FROM --platform=$BUILDPLATFORM alpine:3.21 AS certs
RUN apk --no-cache add ca-certificates

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY flink-server /flink-server

WORKDIR /data
VOLUME ["/data"]
EXPOSE 8080

ENTRYPOINT ["/flink-server"]
CMD ["--config", "/data/flink.yaml"]
