(async function(){
  const debugRefreshBtn = document.getElementById("debugRefresh");
  const debugData = document.getElementById("debugData");
  if(debugRefreshBtn) {
    debugRefreshBtn.addEventListener("click", async function(){
      debugData.textContent = "Fetching debug info...";
      try {
        const resp = await fetch("/api/debug");
        if(!resp.ok) {
          debugData.textContent = "Error: " + (await resp.text());
          return;
        }
        const data = await resp.json();
        debugData.textContent = JSON.stringify(data, null, 2);
      } catch(e){
        debugData.textContent = "Fetch error: " + e;
      }
    });
  }
})();
