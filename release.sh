#!/bin/bash
set -e

# version and keys are supplied as arguments
version="$1"
rc=`echo $version | awk -F - '{print $2}'`
keyfile="$2"
pubkeyfile="$3" # optional
if [[ -z $version || -z $keyfile ]]; then
	echo "Usage: $0 VERSION KEYFILE"
	exit 1
fi
if [[ -z $pubkeyfile ]]; then
	echo "Warning: no public keyfile supplied. Binaries will not be verified."
fi

# check for keyfile before proceeding
if [ ! -f $keyfile ]; then
    echo "Key file not found: $keyfile"
    exit 1
fi

# setup build-time vars
ldflags="-s -w -X 'gitlab.com/scpcorp/ScPrime/build.GitRevision=`git rev-parse --short HEAD`' -X 'gitlab.com/scpcorp/ScPrime/build.BuildTime=`date`' -X 'gitlab.com/scpcorp/ScPrime/build.ReleaseTag=${rc}'"

for arch in amd64 arm; do
	for os in darwin linux windows; do

		# We don't need ARM versions for windows or mac (just linux)
		if [ "$arch" == "arm" ]; then
			if [ "$os" == "windows" ] || [ "$os" == "darwin" ]; then
				continue
			fi
		fi

		echo Packaging ${os} ${arch}...

		# create workspace
		folder=release/ScPrime-$version-$os-$arch
		rm -rf $folder
		mkdir -p $folder
		# compile and sign binaries
		for pkg in spc spd; do
			bin=$pkg
			if [ "$os" == "windows" ]; then
				bin=${pkg}.exe
			fi
			GOOS=${os} GOARCH=${arch} go build -a -tags 'netgo' -ldflags="$ldflags" -o $folder/$bin ./cmd/$pkg

		done

		# add other artifacts
		cp -r doc LICENSE README.md $folder
		# zip
		(
			cd release
			zip -rq ScPrime-$version-$os-$arch.zip ScPrime-$version-$os-$arch
      # import key from $keyfile to gpg keys
			gpg --import $keyfile
			# sign zip release
			gpg --armour --output ScPrime-$version-$os-$arch.zip.asc --detach-sig ScPrime-$version-$os-$arch.zip
		)
	done
done

