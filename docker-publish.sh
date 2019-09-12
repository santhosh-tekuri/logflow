#!/usr/bin/env bash

set -e
if [[ $# -ne 1 ]]; then
    echo "Usage: docker-publish.sh TAG"
    exit 1
fi
name=santhoshkt/logflow
image=$name:$1
latest=$name:latest

cat <<EOF > Dockerfile
FROM scratch
COPY logflow /
ENTRYPOINT ["/logflow"]
EOF
trap "rm -f Dockerfile logflow" EXIT

archs=(amd64 arm arm64 ppc64le s390x)
declare -a images
for arch in "${archs[@]}"; do
    echo bulding ${image}-${arch} ----------------------
    rm -f logflow
    docker run --rm -v "$PWD":/logflow -w /logflow -e GOARCH=${arch} -e CGO_ENABLED=0 golang:1.13.0 go build -a
    docker build -t ${image}-${arch} .
    docker push ${image}-${arch}
    images+=(${image}-${arch})
done

echo buildings manifests ----------------------
docker manifest create --amend ${image} ${images[@]}
docker manifest create --amend ${latest} ${images[@]}
for arch in "${archs[@]}"; do
  docker manifest annotate ${image} ${image}-${arch} --os linux --arch ${arch}
  docker manifest annotate ${latest} ${image}-${arch} --os linux --arch ${arch}
done
docker manifest inspect ${image}
docker manifest push --purge ${image}
docker manifest inspect ${latest}
docker manifest push --purge ${latest}
