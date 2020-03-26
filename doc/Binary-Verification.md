## Verifying Release Signatures

If a verification step fails, please contact https://scpri.me/support/ for
additional information or to find a support contact before using the binary 

1. First you need to download and import the correct `gpg` key. This key will not be changed without advanced notice.
  - `wget -c https://gitlab.com/scpcorp/ScPrime/raw/master/doc/developer-pubkeys/sia-signing-key.asc`
  - `gpg --import sia-signing-key.asc`

2. Download the `SHA256SUMS` file for the release.

3. Verify the signature.
   
   **If the output of that command fails STOP AND DO NOT USE THAT BINARY.**

4. Hash your downloaded binary.
  - `shasum256 ./spd` or similar, providing the path to the binary as the first argument.
	 
   **If the output of that command is not found in `ScPrime-1.x.x-SHA256SUMS.txt.asc` STOP AND DO NOT USE THAT BINARY.**
