(async function(){
  const xummLoginBtn = document.getElementById("xummLoginBtn");
  const xummLoginContainer = document.getElementById("xummLoginContainer");
  const createSection = document.getElementById("createSection");
  if(xummLoginBtn) {
    xummLoginBtn.addEventListener("click", async function(){
      xummLoginContainer.textContent = "Requesting XUMM login QR...";
      try {
        const xumm = new XummSDK();
        const payload = await xumm.payload.createAndSubscribe({ "TransactionType": "SignIn" });
        if (payload && payload.data && payload.data.refs && payload.data.refs.qr_png) {
          xummLoginContainer.innerHTML = `<img src="${payload.data.refs.qr_png}" style="max-width:300px;" alt="XUMM Login QR"/><p>Scan with your XUMM app to sign in.</p>`;
          createSection.style.display = "block";
        } else {
          xummLoginContainer.textContent = "Unexpected payload response: " + JSON.stringify(payload, null, 2);
        }
      } catch(e) {
        xummLoginContainer.textContent = "XUMM login error: " + e;
      }
    });
  }
})();
