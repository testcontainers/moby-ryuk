TARGETIMAGE=${1:-target/image:ci}
IS_RELEASE=${2:-no}
WINBASE="mcr.microsoft.com/windows/nanoserver"
OSVERSIONS=("1809" "1903" "1909" "ltsc2019" "2004" "20H2" "ltsc2022")
MANIFESTLIST=""
BUILDX_PUSH=""

if [ "$IS_RELEASE" = "yes" ]; then
  export BUILDX_PUSH="--push";
fi;

echo "Building for Linux"
docker buildx build \
  --platform linux/amd64,linux/arm64,linux/ppc64le,linux/s390x,linux/386,linux/arm/v7,linux/arm/v6 \
  ${BUILDX_PUSH} \
  --pull \
  --target linux \
  -t $TARGETIMAGE \
  .

for VERSION in ${OSVERSIONS[*]}
do
  echo "Building Windows ${VERSION}"
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
# Not sure the remove of the manifest is needed
docker manifest rm $TARGETIMAGE > /dev/null 2>&1
# if you push the Docker images the manifest is not locally
docker pull $TARGETIMAGE
lin_images=$(docker manifest inspect $TARGETIMAGE | jq -r '.manifests[].digest')

echo "Creating Linux manifest: ${lin_images}"
docker manifest create $TARGETIMAGE $MANIFESTLIST ${lin_images//sha256:/${TARGETIMAGE%%:*}@sha256:}

for VERSION in ${OSVERSIONS[*]}
do
  # Not sure the remove of the manifest is needed
  echo "Annotating Windows platforms to the manifest: ${WINBASE}:${VERSION}"
  docker manifest rm ${WINBASE}:${VERSION} > /dev/null 2>&1
  # if you push the Docker images the manifest is not locally
  docker pull ${WINBASE}:${VERSION}
  full_version=$(docker manifest inspect ${WINBASE}:${VERSION} |jq -r '.manifests[]|.platform|."os.version"'| sed 's@.*:@@') || true;
  docker manifest annotate \
    --os-version ${full_version} \
    --os windows \
    --arch amd64 \
    ${TARGETIMAGE} "${TARGETIMAGE}-${VERSION}"
done

if [ "$IS_RELEASE" = "yes" ]; then
  echo "Pushing manifest to $TARGETIMAGE"
  docker manifest push $TARGETIMAGE
fi
