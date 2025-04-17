document.addEventListener("DOMContentLoaded", function(){
  console.log("DOM fully loaded");
  function appendDebugLog(message) {
    const debugConsole = document.getElementById("debugConsole");
    if (debugConsole) {
      debugConsole.textContent += message + "\n";
    }
    console.log("DEBUG:", message);
  }
  const networkSelect = document.getElementById("networkSelect");
  const saveNetworkBtn = document.getElementById("saveNetworkBtn");
  const networkStatus = document.getElementById("networkStatus");
  const walletOptionSection = document.getElementById("walletOptionSection");
  const existingWalletSection = document.getElementById("existingWalletSection");
  const createSection = document.getElementById("createSection");
  const activationSection = document.getElementById("activationSection");
  const backupSection = document.getElementById("backupSection");
  const xummSection = document.getElementById("xummSection");
  const verifySection = document.getElementById("verifySection");
  const currentWalletSection = document.getElementById("currentWalletSection");
  const currentWalletDisplay = document.getElementById("currentWalletDisplay");
  const createAddressBtn = document.getElementById("createAddressBtn");
  const useExistingBtn = document.getElementById("useExistingBtn");
  const clearWalletBtn = document.getElementById("clearWalletBtn");
  const activateBtn = document.getElementById("activateBtn");
  const toggleRecoveryBtn = document.getElementById("toggleRecoveryBtn");
  const createResult = document.getElementById("createResult");
  const existingResult = document.getElementById("existingResult");
  const displayKey = document.getElementById("displayKey");
  const verifyPrompt = document.getElementById("verifyPrompt");
  const verifyInput = document.getElementById("verifyInput");
  const verifyBtn = document.getElementById("verifyBtn");
  const refreshDebug = document.getElementById("refreshDebug");
  const apiKeyInput = document.getElementById("apiKeyInput");
  const apiSecretInput = document.getElementById("apiSecretInput");
  const setXummBtn = document.getElementById("setXummBtn");
  const xummResult = document.getElementById("xummResult");
  const mintNftBtn = document.getElementById("mintNftBtn");
  const mintResult = document.getElementById("mintResult");
  const getXummQRBtn = document.getElementById("getXummQRBtn");
  const deriveAddressBtn = document.getElementById("deriveAddressBtn");
  const seedInput = document.getElementById("seedInput");
  const derivedAddress = document.getElementById("derivedAddress");
  let rippleWallet = "";
  let cloudstormAddress = "";
  let recoveryKey = "";
  let challengeIndices = [];
  async function fetchNetwork() {
    try {
      const resp = await fetch("/api/getNetwork");
      if(!resp.ok) {
        appendDebugLog("Unable to retrieve XRPL network setting.");
        return;
      }
      const data = await resp.json();
      if(data.network) {
        networkSelect.value = data.network;
        networkStatus.textContent = "Current: " + data.network;
      }
    } catch(e) {
      appendDebugLog("Error fetching network: " + e);
    }
  }
  saveNetworkBtn.addEventListener("click", async function(){
    const selected = networkSelect.value;
    try {
      const resp = await fetch("/api/setNetwork", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ network: selected })
      });
      if(!resp.ok) {
        appendDebugLog("Error saving network: " + (await resp.text()));
        return;
      }
      const result = await resp.json();
      networkStatus.textContent = "Current: " + (result.network || "unknown");
      appendDebugLog("XRPL network updated to: " + result.network);
    } catch(e) {
      appendDebugLog("Error setting network: " + e);
    }
  });
  function clearOnFocus(e) {
    e.target.value = "";
  }
  document.querySelectorAll("input[autocomplete='off']").forEach(input => {
    input.addEventListener("focus", clearOnFocus);
  });
  function updateWalletOption() {
    if(optionExisting.checked) {
      existingWalletSection.classList.remove("hidden");
      createAddressBtn.disabled = true;
    } else {
      existingWalletSection.classList.add("hidden");
      createAddressBtn.disabled = false;
    }
  }
  optionNew.addEventListener("change", updateWalletOption);
  optionExisting.addEventListener("change", updateWalletOption);
  updateWalletOption();
  async function checkWalletConnection() {
    try {
      const resp = await fetch("/api/servicedetails");
      if(resp.ok) {
        const details = await resp.json();
        if(details.xrplWallet && details.xrplWallet !== "") {
          rippleWallet = details.xrplWallet;
          appendDebugLog("Ripple Wallet connected: " + rippleWallet);
          currentWalletDisplay.textContent = 
            "Ripple Address (for Xumm import): " + rippleWallet + 
            "\nCloudstorm Address (ledger NFT): " + cloudstormAddress;
          currentWalletSection.classList.remove("hidden");
          walletConnected();
        }
      }
    } catch(e) {
      appendDebugLog("Error checking wallet connection: " + e);
    }
  }
  async function fetchDebugLogs() {
    try {
      const resp = await fetch("/api/console");
      if(resp.ok){
        const logs = await resp.text();
        const debugConsole = document.getElementById("debugConsole");
        if(debugConsole) {
          debugConsole.textContent = logs;
        }
      } else {
        appendDebugLog("Error fetching debug logs.");
      }
    } catch(e){
      appendDebugLog("Exception fetching debug logs: " + e);
    }
  }
  setInterval(fetchDebugLogs, 10000);
  refreshDebug.addEventListener("click", fetchDebugLogs);
  setInterval(checkWalletConnection, 5000);
  async function walletConnected() {
    walletOptionSection.classList.add("hidden");
    createSection.classList.add("hidden");
    existingWalletSection.classList.add("hidden");
    backupSection.classList.remove("hidden");
    xummSection.classList.remove("hidden");
    verifySection.classList.remove("hidden");
    appendDebugLog("Wallet connected. Ripple: " + rippleWallet + " | Cloudstorm: " + cloudstormAddress);
  }
  createAddressBtn.addEventListener("click", async function(){
    createResult.textContent = "Creating Cloudstorm wallet and starting ledger...";
    appendDebugLog("Initiating wallet creation request.");
    try {
      const resp = await fetch("/api/mint", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action: "createAddress" })
      });
      if(!resp.ok){
        createResult.textContent = "Error: " + (await resp.text());
        appendDebugLog("Wallet creation error: " + createResult.textContent);
        return;
      }
      const data = await resp.json();
      if(!data.address || !data.recoveryKey){
        createResult.textContent = "Invalid response from server.";
        appendDebugLog("Invalid wallet creation response.");
        return;
      }
      cloudstormAddress = data.address;
      recoveryKey = data.recoveryKey;
      appendDebugLog("Recovery key generated: " + recoveryKey);
      displayKey.textContent = recoveryKey;
      await checkWalletConnection();
      currentWalletDisplay.textContent =
        "Ripple Address (for Xumm import): " + rippleWallet +
        "\nCloudstorm Address (ledger NFT): " + cloudstormAddress;
      currentWalletSection.classList.remove("hidden");
      walletConnected();
    } catch(e){
      createResult.textContent = "Address creation error: " + e;
      appendDebugLog("Address creation exception: " + e);
    }
  });
  useExistingBtn.addEventListener("click", async function(){
    const existingAddress = document.getElementById("existingAddressInput").value.trim();
    const existingRecovery = document.getElementById("existingRecoveryInput").value.trim();
    if(!existingAddress || !existingRecovery) {
      existingResult.textContent = "Please enter both wallet address and recovery key.";
      return;
    }
    existingResult.textContent = "Using existing wallet...";
    appendDebugLog("Attempting to use existing wallet.");
    try {
      const resp = await fetch("/api/mint", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action: "useWallet", address: existingAddress, recoveryKey: existingRecovery })
      });
      if(!resp.ok){
        existingResult.textContent = "Error: " + (await resp.text());
        appendDebugLog("Error using existing wallet: " + existingResult.textContent);
        return;
      }
      const data = await resp.json();
      if(!data.address || !data.recoveryKey){
        existingResult.textContent = "Invalid response from server.";
        appendDebugLog("Invalid response for existing wallet.");
        return;
      }
      rippleWallet = data.address;
      existingResult.textContent = "Existing wallet accepted: " + rippleWallet;
      recoveryKey = data.recoveryKey;
      appendDebugLog("Loaded recovery key: " + recoveryKey);
      currentWalletDisplay.textContent = "Ripple Address (for Xumm import): " + rippleWallet;
      currentWalletSection.classList.remove("hidden");
      displayKey.textContent = recoveryKey;
      verifySection.classList.remove("hidden");
      walletConnected();
    } catch(e){
      existingResult.textContent = "Error using existing wallet: " + e;
      appendDebugLog("Existing wallet exception: " + e);
    }
  });
  clearWalletBtn.addEventListener("click", async function(){
    if(!confirm("Are you sure you want to clear the wallet? This action may cause loss of funds if performed incorrectly.")) return;
    try {
      const resp = await fetch("/api/clearwallet", { method: "POST" });
      if(!resp.ok) {
        appendDebugLog("Error clearing wallet: " + (await resp.text()));
        return;
      }
      appendDebugLog("Wallet cleared successfully.");
      location.reload();
    } catch(e) {
      appendDebugLog("Exception clearing wallet: " + e);
    }
  });
  activateBtn.addEventListener("click", async function(){
    try {
      const reserveResp = await fetch("/api/reserve");
      if(!reserveResp.ok) throw new Error("Failed to fetch account reserve");
      const reserveData = await reserveResp.json();
      document.getElementById("reserveInfo").textContent = "Current Reserve: " + reserveData.reserve;
      if(reserveData.reserve < reserveData.required) {
        document.getElementById("activationResult").textContent = "Wallet not activated. Ensure sufficient XRP.";
      } else {
        document.getElementById("activationResult").textContent = "Wallet activated successfully.";
        activationSection.classList.add("hidden");
        backupSection.classList.remove("hidden");
      }
    } catch(e) {
      document.getElementById("activationResult").textContent = "Error checking activation: " + e;
    }
  });
  toggleRecoveryBtn.addEventListener("click", function(){
    if(displayKey.classList.contains("hidden")){
      displayKey.classList.remove("hidden");
      toggleRecoveryBtn.textContent = "Hide Recovery Key";
    } else {
      displayKey.classList.add("hidden");
      toggleRecoveryBtn.textContent = "Show Recovery Key";
    }
  });
  verifyBtn.addEventListener("click", function(){
    const input = verifyInput.value.trim().toLowerCase();
    const words = recoveryKey.split(" ");
    const expected = challengeIndices.map(i => words[i]).join(" ");
    if(input === expected){
      verifyResult.textContent = "Recovery key confirmed.";
      appendDebugLog("Recovery key verified successfully.");
    } else {
      verifyResult.textContent = "Incorrect words entered. Please try again.";
      appendDebugLog("Recovery key verification failed. Expected: " + expected + " but got: " + input);
    }
  });
  setXummBtn.addEventListener("click", function(){
    const xummApiKey = apiKeyInput.value.trim();
    const xummApiSecret = apiSecretInput.value.trim();
    if(xummApiKey && xummApiSecret){
      xummResult.textContent = "Xumm API credentials set.";
      appendDebugLog("Xumm API credentials set.");
    } else {
      xummResult.textContent = "Please enter both API key and secret.";
      appendDebugLog("Xumm API credentials missing.");
    }
  });
  getXummQRBtn.addEventListener("click", async function(){
    try {
      const resp = await fetch("/api/xumm/sign", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ wallet: rippleWallet })
      });
      if (!resp.ok) {
        appendDebugLog("Error initiating Xaman QR code generation: " + (await resp.text()));
        return;
      }
      const sessionData = await resp.json();
      if(sessionData.qrCode){
        document.getElementById("xummQRContainer").innerHTML = `<img src="data:image/png;base64,${sessionData.qrCode}" alt="Xaman QR Code">`;
      } else {
        appendDebugLog("No QR code returned from server.");
      }
      appendDebugLog("Xaman QR code generated. Scan with your app to import the wallet.");
    } catch (e) {
      appendDebugLog("Exception during QR code generation: " + e);
    }
  });
  deriveAddressBtn.addEventListener("click", async function(){
    const seed = seedInput.value.trim();
    if(!seed) {
      alert("Please enter a family seed.");
      return;
    }
    try {
      const resp = await fetch("/api/deriveAddress", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ seed })
      });
      if(!resp.ok) {
        appendDebugLog("Error deriving address: " + (await resp.text()));
        return;
      }
      const data = await resp.json();
      derivedAddress.textContent = "Derived XRP Address: " + data.address;
      appendDebugLog("Derived address: " + data.address);
    } catch(e) {
      appendDebugLog("Exception deriving address: " + e);
    }
  });
  fetchNetwork();
  checkWalletConnection();
  fetchDebugLogs();
});