function refreshBlockHeight() {
  if (document.getElementsByClassName('block_height').length > 0) {
    fetch("/gui/blockHeight")
      .then(response => response.json())
      .then(result => {
        var blockHeight = result[0];
        var status = result[1]
        var color = result[2]
        // Autorefresh wallet to make onboarding smoother.
        if (status === "Synchronized") {
          var pathname = location.pathname
          if (pathname === "/gui/initializeSeed" || pathname === "/gui/restoreSeed") {
            document.getElementById("refreshScanner").submit(); 
          }
        }
        for (const element of document.getElementsByClassName("block_height")){
          element.innerHTML=blockHeight;
        }
        for (const element of document.getElementsByClassName("status")){
          element.innerHTML=status;
        }
        for (const element of document.getElementsByClassName("status")){
          element.className="status " + color
        }
        setTimeout(() => {refreshBlockHeight();}, 5000);
      })
      .catch(error => {
        console.error("Error:", error);
        setTimeout(() => {refreshBlockHeight();}, 5000);
      })
  } else {
    setTimeout(() => {refreshBlockHeight();}, 5000);
  }
}
function refreshBalance() {
  if (document.getElementsByClassName('balance').length > 0) {
    fetch("/gui/balance")
      .then(response => response.json())
      .then(result => {
        for (const element of document.getElementsByClassName("confirmed")){
          element.innerHTML=result[0];
        }
        for (const element of document.getElementsByClassName("unconfirmed")){
          element.innerHTML=result[1];
        }
        for (const element of document.getElementsByClassName("spf_funds")){
          element.innerHTML=result[2];
        }
        setTimeout(() => {refreshBalance();}, 5000);
      })
      .catch(error => {
        console.error("Error:", error);
        setTimeout(() => {refreshBalance();}, 5000);
      })
  } else {
    setTimeout(() => {refreshBalance();}, 5000);
  }
}
function refreshDownloaderProgress() {
  if (document.getElementsByClassName('downloader-progress').length > 0) {
    fetch("/gui/downloaderProgress")
      .then(response => response.json())
      .then(result => {
        var status = result[0]
        // Autorefresh wallet to make onboarding smoother.
        if (status === "100%") {
          var pathname = location.pathname
          if (pathname === "/") {
            document.getElementById("refreshDownloader").submit(); 
          }
        }
        for (const element of document.getElementsByClassName("downloader-progress")){
          element.innerHTML = status;
        }
        setTimeout(() => {refreshDownloaderProgress();}, 1000);
      })
      .catch(error => {
        console.error("Error:", error);
        setTimeout(() => {refreshDownloaderProgress();}, 1000);
      })
  } else {
    setTimeout(() => {refreshDownloaderProgress();}, 1000);
  }
}
function refreshHeartbeat() {
  fetch("/gui/heartbeat")
    .then(response => response.json())
    .then(result => {
    	if (result[0] === "true") {
        setTimeout(() => {refreshHeartbeat();}, 200);
    	}
    })
    .catch(error => {
      shutdownNotice()
    })
}
function shutdownNotice() {
  document.body.innerHTML = `
    <div class="col-5 left top no-wrap">
      <div>
        <img class="scprime-logo" alt="ScPrime Wallet" src="/gui/logo.png"/>
      </div>
    </div>
    <div id="popup" class="popup center">
      <h2 class="uppercase">Shutdown Notice</h2>
      <div class="middle pad blue-dashed" id="popup_content">Wallet was shutdown.</div>
    </div>
    <div id="fade" class="fade"></div>
  `
}
refreshDownloaderProgress()
refreshBlockHeight()
refreshBalance()
refreshHeartbeat()
