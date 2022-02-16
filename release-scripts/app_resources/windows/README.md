# Overview
Appifying the windows scp-ui build is multifaceted.

First, the easiest way to inject an icon and metadata into a windows binary in Golang is to generate `rsrc_windows_386.syso` and `rsrc_windows_amd64.syso` files. This is because Golang will automatically inject the `rsrc_windows_386.syso` or `rsrc_windows_amd64.syso` file that is included in the target package into the final windows binary. Thus, before building the windows scp-ui binary we need to copy `rsrc_windows_386.syso` and `rsrc_windows_amd64.syso` from this directory into the `./cmd/scp-ui/` directory.

Second, when building the windows scp-ui binary we need to tell Golang to build an application binary instead of a command line binary. This is accomplished by suplying `-H windowsgui` as an additional ldflags parameter in the build command. The main benefit of this is that double-clicking on the application will not launch the command terminal.

# Editing
To edit the metadata edit the `./winres/winres.json` file. To change the large icon replace the `./winres/icon.png` file with your desired icon. Likewise, to change the small icon replace `./winres/icon16.png` with your desired icon. Also, to edit the metadata or the icons we need to compile new `syso` files. To do this I recommend downloading go-winres to this directory from https://github.com/tc-hib/go-winres and then running `./go-winres make` from this directory in your terminal.

