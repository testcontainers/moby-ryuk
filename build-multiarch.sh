TARGETIMAGE=${1:-target/image:ci}
IS_RELEASE=${2:-no}
WINBASE="mcr.microsoft.com/windows/nanoserver"
OSVERSIONS=("1809" "1903" "1909" "ltsc2019" "2004" "20H2" "ltsc2022")
MANIFESTLIST=""
BUILDX_PUSH=""

if [ "$IS_RELEASE" = "yes" ]; then
  export BUILDX_PUSH="--push";
fi;

# build for Linux
docker buildx build \
  --platform linux/amd64,linux/arm64,linux/ppc64le,linux/s390x,linux/386,linux/arm/v7,linux/arm/v6 \
  ${BUILDX_PUSH} \
  --pull \
  --target linux \
  -t $TARGETIMAGE \
  .

# build for Windows
for VERSION in ${OSVERSIONS[*]}
do
    docker buildx build \
      --platform windows/amd64 \
      ${BUILDX_PUSH} \
      --pull \
      --build-arg WINBASE=${WINBASE}:${VERSION} \
      --target windows \
      -t "${TARGETIMAGE}-${VERSION}" \
      .
    MANIFESTLIST+="${TARGETIMAGE}-${VERSION} "
done

# Get images from Linux manifest list, append and annotate Windows images and overwrite in registry
docker manifest rm $TARGETIMAGE > /dev/null 2>&1
lin_images=$(docker manifest inspect $TARGETIMAGE | jq -r '.manifests[].digest')

docker manifest create $TARGETIMAGE $MANIFESTLIST ${lin_images//sha256:/${TARGETIMAGE%%:*}@sha256:}

for VERSION in ${OSVERSIONS[*]}
do
  docker manifest rm ${WINBASE}:${VERSION} > /dev/null 2>&1
  full_version=`docker manifest inspect ${WINBASE}:${VERSION} | grep "os.version" | head -n 1 | awk '{print $$2}' | sed 's@.*:@@' | sed 's/"//g'` || true;
  docker manifest annotate \
    --os-version ${full_version} \
    --os windows \
    --arch amd64 \
    ${TARGETIMAGE} "${TARGETIMAGE}-${VERSION}"
done

if [ "$IS_RELEASE" = "yes" ]; then
  docker manifest push $TARGETIMAGE
fi
