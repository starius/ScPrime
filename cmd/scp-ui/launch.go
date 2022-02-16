package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Browsers that are based on Chromium (such as Google Chrome and Microsoft Edge) are most
// desirable because they can be launched in app mode (which means that there is no address bar).
// This allows the GUI head feel most like a native application.
func launch(url string) bool {
	time.Sleep(500 * time.Millisecond)
	switch runtime.GOOS {
	case "android":
		return android(url)
	case "darwin":
		return darwin(url)
	case "windows":
		return windows(url)
	}
	return nix(url)
}

func android(url string) bool {
	err := exec.Command("am", "start", "--user", "0", "-a", "android.intent.action.VIEW", "-d", url).Run() // #nosec
	return err == nil
}

func darwin(url string) bool {
	err := exec.Command("open", "-n", "-a", "Google Chrome", "--args", "--app="+url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("open", "-n", "-a", "Microsoft Edge", "--args", "--app="+url).Run() // #nosec
	if err == nil {
		return true
	}
	// Firefox can no longer be launched without an address bar as the ssb parameter (site
	// specific browser) was removed in 2021.
	err = exec.Command("open", "-n", "-a", "Firefox", "--args", "-new-window="+url).Run() // #nosec
	if err == nil {
		return true
	}
	// Use Safari as a last resort as if it is not the default browser then two GUI heads will
	// open (one in Safari and one in the default browser).
	err = exec.Command("open", "-a", "Safari", url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("open", url).Run() // #nosec
	return err == nil
}

func windows(url string) bool {
	url = strings.Replace(url, "[::]", "[::1]", 1)
	err := exec.Command("C:/Users/"+os.Getenv("USERNAME")+"/AppData/Local/Google/Chrome/Application/chrome.exe", "--app="+url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("C:/Program Files/Google/Chrome/Application/chrome.exe", "--app="+url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("C:/Program Files (x86)/Google/Chrome/Application/chrome.exe", "--app="+url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("C:/Users/"+os.Getenv("USERNAME")+"/AppData/Local/Microsoft/Edge/Application/msedge.exe", "--app="+url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("C:/Program Files/Microsoft/Edge/Application/msedge.exe", "--app="+url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe", "--app="+url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("C:/Users/"+os.Getenv("USERNAME")+"/AppData/Local/Mozilla Firefox/firefox.exe", "-new-window", url).Run() // #nosec
	if err == nil {
		return true
	}
	// Firefox can no longer be launched without an address bar as the ssb parameter (site
	// specific browser) was removed in 2021.
	err = exec.Command("C:/Program Files/Mozilla Firefox/firefox.exe", "-new-window", url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("C:/Program Files (x86)/Mozilla Firefox/firefox.exe", "-new-window", url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command(filepath.Join(os.Getenv("SYSTEMROOT"), "System32", "rundll32.exe"), "url.dll,FileProtocolHandler", url).Run() // #nosec
	return err == nil
}

func nix(url string) bool {
	err := exec.Command("google-chrome", "--app="+url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("google-chrome-stable", "--app="+url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("chromium", "--app="+url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("chromium-browser", "--app="+url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("/usr/bin/google-chrome", "--app="+url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("/usr/bin/google-chrome-stable", "--app="+url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("/usr/bin/chromium", "--app="+url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("/usr/bin/chromium-browser", "--app="+url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("microsoft-edge", "--app="+url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("microsoft-edge-stable", "--app="+url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("/usr/bin/microsoft-edge", "--app="+url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("/usr/bin/microsoft-edge-stable", "--app="+url).Run() // #nosec
	if err == nil {
		return true
	}
	// Firefox can no longer be launched without an address bar as the ssb parameter (site
	// specific browser) was removed in 2021.
	err = exec.Command("firefox", "--new-window", url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("/usr/bin/firefox", "--new-window", url).Run() // #nosec
	if err == nil {
		return true
	}
	err = exec.Command("xdg-open", url).Run() // #nosec
	return err == nil
}
