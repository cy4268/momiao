FROM alpine:3.22.5@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --chown=65532:65532 bin/ /app/
RUN chmod 0555 /app/momiao /app/momiao-migrate /app/momiao-import-native
COPY --chown=65532:65532 web/ /app/web/
COPY --chown=65532:65532 licenses/ /app/licenses/
ARG REVISION
LABEL org.opencontainers.image.source="https://github.com/cy4268/momiao" org.opencontainers.image.revision="$REVISION"
USER 65532:65532
CMD ["/app/momiao"]
