# Pubaccesskey Manager
The `pubaccesskey` package defines Pubaccesskeys used for encrypting files in Pubaccess and
provides a way to persist Pubaccesskeys using a `SkykeyManager` that manages these
keys in a file on-disk.

The file consists of a header which is:
  `SkykeyFileMagic | SkykeyVersion | Length`

The `SkykeyFileMagic` never changes. The version only changes when
backwards-incompatible changes are made to the design of `Pubaccesskeys` or to the way
this file is structured. The length refers to the number of bytes in the file.

When adding a `Pubaccesskey` to the file, a `Pubaccesskey` is unmarshaled and appended to
the end of the file and the file is then synced. Then the length field in the
header is updated to indicate the newly written bytes and the file is synced
once again.

## Pubaccesskeys
A `Pubaccesskey` is a key associated with a name to be used in Pubaccess to share
encrypted files. Each key has a name and a unique identifier.
