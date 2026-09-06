FROM calciumion/new-api@sha256:54a0b10924aa75fa5b5947208b820ced66b6ef4b445b35f122b31d80676aba2b
LABEL org.opencontainers.image.revision="9520310d6d75feb0c30c70aaafe835e83b8ccff2" \
      win.momiao.native.upstream="f116414284162ad15d8925f7bca494c109b83e93" \
      win.momiao.native.tree="3c15a618fa7a528c06da92de7dbf2f2c843a9162"
COPY --chmod=0555 new-api /new-api
COPY --chmod=0555 momiao-admission-migrate /usr/local/bin/momiao-admission-migrate
ENV REDIS_CONN_STRING="" BATCH_UPDATE_ENABLED=false MOMIAO_ADMISSION_ENABLED=false
USER 65534:65534
WORKDIR /data
EXPOSE 3000
ENTRYPOINT ["/new-api"]
