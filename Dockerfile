FROM golang:1.12.7 as gobuild
WORKDIR /go/src/github.com/santhosh-tekuri/logflow
COPY . .
RUN go get -d ./...
RUN CGO_ENABLED=0 go install -a

FROM scratch
COPY --from=gobuild /go/bin/logflow /
ENTRYPOINT ["/logflow"]

