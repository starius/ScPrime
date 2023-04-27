#!/usr/bin/env bash

# This script runs the devnet.
#
# It runs "make dev" and puts binaries into .devnet/bin/ and then uses them to
# run nodes.
#
# By default it runs 3 nodes using 428x, 528x and 628x sets of ports and uses
# .devnet/nodes/X for metadata (where X is the node number).
# First node starts mining blocks right away. Its wallet will have most of the
# available SCP and SPFs. All wallets will be unlocked from the start.
# Seed for the first node is in the variable $SEED defined below,
# other seeds will be printed to the console.
#
# Run this script with the `-h` to see available options.

set -e

# Stop background commands (spd) when main script finishes.
trap "trap - SIGTERM && kill -- -$$" SIGINT SIGTERM EXIT

# Seed that will give you most of the SCP and SPFs on a dev build.
SEED="hookup under total towel equip deity wrong nuisance lectures musical waking succeed pouch corrode damp butter dime hacksaw snake haunted fuzzy alchemy dagger tasked hemlock soccer needed hijack academy"

# Current dir.
SCRIPT_DIR=$(cd -- "$( dirname -- "${BASH_SOURCE[0]}")" &> /dev/null && pwd)
HOME_DIR="${SCRIPT_DIR}/.."
WORKING_DIR="${HOME_DIR}/.devnet"
NODES_DIR="${WORKING_DIR}/nodes"
MODULES_MAIN="gctwhrm"
MODULES_SECOND="gctw"

startNode() {
  NODE=${1-1}
  if [[ $NODE == "1" ]]; then
    echo "-- Starting node $NODE (Main)"
    MODULES="$MODULES_MAIN"
    BOOTSTRAP_ARG="--no-bootstrap" # Main node does not need bootstrap.
  else
    echo "-- Starting node $NODE (Secondary)"
    MODULES="$MODULES_SECOND"
    BOOTSTRAP_ARG=""
  fi

  API_PORT=$(getAPIPort $NODE)
  $SPD -M "${MODULES}" \
    --scprime-directory "$(getNodeDir $NODE)" \
    --host-addr=localhost:$(($API_PORT+2)) \
    --rpc-addr=localhost:$(($API_PORT+1)) \
    --api-addr=localhost:$API_PORT \
    --siamux-addr=localhost:$(($API_PORT+5)) \
    --siamux-addr-ws=localhost:$(($API_PORT+4)) \
    --host-api-addr=localhost:$(($API_PORT+3)) \
    "${BOOTSTRAP_ARG}" | sed "s/^/[$NODE] /" \
    &

  # Wait till node starts responding.
  until $(curl --fail-with-body -s -u ":$(getAPIPassword)" -A "ScPrime-Agent" \
    $(getAPIAddress $NODE)/consensus -o /dev/null); do sleep 1; done
  until $(curl --fail-with-body -s -u ":$(getAPIPassword)" -A "ScPrime-Agent" \
    $(getAPIAddress $NODE)/gateway -o /dev/null); do sleep 1; done
  until $(curl --fail-with-body -s -u ":$(getAPIPassword)" -A "ScPrime-Agent" \
    $(getAPIAddress $NODE)/wallet -o /dev/null); do sleep 1; done

  spc=$(spcCmd $NODE)
  echo "-- spc command to use with this node:"
  echo "# ${spc#"$HOME_DIR/"}"

  if [[ $NODE == "1" ]]; then
    curl --fail-with-body -s -u ":$(getAPIPassword)" -A "ScPrime-Agent" \
      $(getAPIAddress $NODE)/wallet/init/seed -X POST -d "seed=${SEED}"
    curl --fail-with-body -s -u ":$(getAPIPassword)" -A "ScPrime-Agent" \
      $(getAPIAddress $NODE)/wallet/unlock -X POST -d "encryptionpassword=${SEED}"
    if [[ -z "$DISABLE_MINER" ]]; then
      curl --fail-with-body -s -u ":$(getAPIPassword)" -A "ScPrime-Agent" \
        $(getAPIAddress $NODE)/miner/start -X GET
    fi
  else
    seed=$(curl --fail-with-body -s -u ":$(getAPIPassword)" -A "ScPrime-Agent" \
      $(getAPIAddress $NODE)/wallet/init -X POST | jq -r '.primaryseed')
    echo "Seed: $seed"
    SCPRIME_WALLET_PASSWORD="$seed" $spc wallet unlock | sed "s/^/[$NODE] /"
    $spc gateway connect 127.0.0.1:$(($(getAPIPort 1)+1)) | sed "s/^/[$NODE] /"
  fi

  echo
}

getNodeDir() {
  echo "${NODES_DIR}/$1"
}

getAPIPort() {
  # 1 -> 4280, 2 -> 5280
  echo $((($1+3) * 1000 + 280))
}

getAPIAddress() {
  echo "localhost:$(getAPIPort $1)"
}

getAPIPassword() {
  os=$(uname -s)
  case "${os}" in
      Linux*)  cat ~/.scprime/apipassword;;
      Darwin*) cat ~/Library/Application\ Support/ScPrime/apipassword;;
      *)       echo "Unknown os: $os" && exit 1;;
  esac
}

spcCmd() {
  echo "$SPC --addr $(getAPIAddress $1)"
}

# Parameters.
help() {
  echo "Usage: run-devnet.sh [-h] [-d] [-c] [-v] [-n <nodes>]"
  echo "  -h          Print help"
  echo "  -d          Disable miner module, by default it starts mining after"
  echo "              the start"
  echo "  -c          Clear spd directories"
  echo "  -n <nodes>  Define number of nodes to run, default is 3."
  echo "              First node starts with modules '${MODULES_MAIN}', second and"
  echo "              subsequent uses '${MODULES_SECOND}'. First node uses default ports,"
  echo "              second uses 528x, third 628x etc."
}

DISABLE_MINER=""
NODES=3
while getopts "dn:ch" opt; do
  case "$opt" in
    d)
      DISABLE_MINER="y"
      ;;
    n)
      NODES="$OPTARG"
      if [[ $NODES -lt 1 ]]; then
        echo "Number of nodes cannot be less than 1"
        exit 1
      fi
      ;;
    c)
      echo "Clearing spd directories: $NODES_DIR"
      rm -rf $NODES_DIR
      echo
      ;;
    ?|h)
      help
      exit 1
      ;;
  esac
done

# Check for required tools.
requireCommand() {
  if ! command -v $1 &> /dev/null; then
    echo "command not found: $1"
    exit 1
  fi
}

requireCommand jq
requireCommand curl

# Build.
GOBIN="${WORKING_DIR}/bin"
echo "-- Building into $GOBIN (spd and spc)"
pushd $SCRIPT_DIR/.. >/dev/null
GOBIN="$GOBIN" make dev
popd >/dev/null
SPD="$WORKING_DIR/bin/spd"
SPC="$WORKING_DIR/bin/spc"
echo

# Run nodes.
for i in $(seq 1 $NODES); do
  startNode $i
done

# Wait indefinitely.
while true; do sleep 600; done
