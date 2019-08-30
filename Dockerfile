FROM golang:1.12.9 as gobuild
WORKDIR /logflow
COPY . .
RUN CGO_ENABLED=0 go install -a

FROM scratch
COPY --from=gobuild /go/bin/logflow /
ENTRYPOINT ["/logflow"]

