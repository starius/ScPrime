#!/usr/bin/env bash
set -e

# version and keys are supplied as arguments
version="$1"
rc=`echo $version | awk -F - '{print $2}'`
if [[ -z $version ]]; then
	echo "Usage: $0 VERSION"
	exit 1
fi

# setup build-time vars
sharedldflags="-s -w -X 'gitlab.com/scpcorp/ScPrime/build.GitRevision=`git rev-parse --short HEAD`' -X 'gitlab.com/scpcorp/ScPrime/build.BuildTime=`git show -s --format=%ci HEAD`' -X 'gitlab.com/scpcorp/ScPrime/build.ReleaseTag=${rc}'"

function build {
  os=$1
  arch=$2

	echo Building ${os}...
	# create workspace
	folder=release/ScPrime-$version-$os-$arch
	rm -rf $folder
	mkdir -p $folder
	# compile and hash binaries
	for pkg in spc spd scp-ui; do
		bin=$pkg
		ldflags=$sharedldflags
		if [ "$os" == "windows" ]; then
			bin=${pkg}.exe
		fi
		# Appify the scp-ui windows release. More documentation at `./release-scripts/app_resources/windows/README.md`.
		if [ "$os" == "windows" ] && [ ${pkg} == "scp-ui" ]; then
			# copy metadata files to the package
			cp ./release-scripts/app_resources/windows/rsrc_windows_386.syso ./cmd/scp-ui/rsrc_windows_386.syso
			cp ./release-scripts/app_resources/windows/rsrc_windows_amd64.syso ./cmd/scp-ui/rsrc_windows_amd64.syso
			# on windows build an application binary instead of a command line binary.
			ldflags="$sharedldflags -H windowsgui"
		fi
		# Appify the scp-ui darwin release. More documentation at `./release-scripts/app_resources/darwin/README.md`.
		if [ "$os" == "darwin" ] && [ ${pkg} == "scp-ui" ]; then
			# copy the scp-ui.app template container into the release directory
			cp -a ./release-scripts/app_resources/darwin/scp-ui.app $folder/$bin.app
			# touch the scp-ui.app container to reset the time created timestamp
			touch $folder/$bin.app
			# set the build target to be inside the scp-ui.app container
			bin=scp-ui.app/Contents/MacOS/scp-ui.app
		fi
		GOOS=${os} GOARCH=${arch} go build -a -tags 'netgo' -trimpath -ldflags="$ldflags" -o $folder/$bin ./cmd/$pkg
		# Cleanup scp-ui windows release.
		if [ "$os" == "windows" ] && [ ${pkg} == "scp-ui" ]; then
			rm ./cmd/scp-ui/rsrc_windows_386.syso
			rm ./cmd/scp-ui/rsrc_windows_amd64.syso
		fi
		(
			cd release/
			sha256sum ScPrime-$version-$os-$arch/$bin >> ScPrime-$version-SHA256SUMS.txt
		)
  done

	cp -r doc LICENSE README.md $folder
}

# Build amd64 binaries.
for os in darwin linux windows; do
  build "$os" "amd64"
done

# Build Raspberry Pi binaries.
build "linux" "arm64"
