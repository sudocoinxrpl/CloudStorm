(async function(){
  const mintSource = document.getElementById("mintSource");
  const mintArchive = document.getElementById("mintArchive");
  const mintTxId = document.getElementById("mintTxId");
  const mintBtn = document.getElementById("mintSubmit");
  const mintResult = document.getElementById("mintResult");
  if(mintBtn) {
    mintBtn.addEventListener("click", async function(){
      mintResult.textContent = "Minting...";
      try {
        const sourceDir = mintSource.value.trim();
        const archiveFile = mintArchive.value.trim();
        const txId = mintTxId.value.trim() || "TX-DEFAULT";
        const resp = await fetch("/api/mint", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ sourceDir: sourceDir, archiveFile: archiveFile, txId: txId })
        });
        if(!resp.ok){
          mintResult.textContent = "Error: " + (await resp.text());
          return;
        }
        const blob = await resp.blob();
        const url = URL.createObjectURL(blob);
        mintResult.innerHTML = "<img src=\"" + url + "\" style=\"max-width:300px;\" alt=\"NFT Card\" /><p>Mint success!</p>";
      } catch(e){
        mintResult.textContent = "Mint error: " + e;
      }
    });
  }
})();
