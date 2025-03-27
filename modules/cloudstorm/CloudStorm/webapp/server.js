"use strict";

const fs = require("fs");
const https = require("https");
const express = require("express");
const cors = require("cors");
const axios = require("axios");
const helmet = require("helmet");
const morgan = require("morgan");
const path = require("path");
const { create } = require("ipfs-http-client"); // Updated import for ipfs-http-client

const app = express();
const passphrase = fs.readFileSync("certs/passphrase.txt", "utf8").trim();
const NETWORK_FILE = "network.txt";

// Middleware configuration
app.use(helmet());
app.use(cors());
app.use(express.json());
app.use(morgan("combined"));
app.use((req, res, next) => {
  const secure = req.secure || req.headers["x-forwarded-proto"] === "https";
  const isLocal = req.hostname === "localhost" || req.hostname === "127.0.0.1";
  if (secure || isLocal) {
    res.setHeader("Cross-Origin-Opener-Policy", "same-origin");
  }
  res.setHeader(
    "Content-Security-Policy",
    "default-src 'self' 'unsafe-inline' https://unpkg.com; img-src 'self' data: https://unpkg.com"
  );
  next();
});

// Serve frontend static files (info.html is the landing page)
app.use(express.static(path.join(__dirname, "public"), { index: "info.html" }));

// Environment variable for the backend Go server (which implements raft, governance, etc.)
const BACKEND_URL = process.env.BACKEND_URL || "http://localhost:5115";

// -------------------------
// Network endpoints
// -------------------------
app.get("/api/getNetwork", async (req, res) => {
  try {
    let network = "mainnet";
    if (fs.existsSync(NETWORK_FILE)) {
      network = fs.readFileSync(NETWORK_FILE, "utf8").trim() || "mainnet";
    }
    res.json({ network });
  } catch (e) {
    console.error("Error in /api/getNetwork:", e);
    res.status(500).json({ error: e.message });
  }
});

app.post("/api/setNetwork", async (req, res) => {
  try {
    const { network } = req.body;
    if (!network) {
      return res.status(400).json({ error: "Network not provided" });
    }
    fs.writeFileSync(NETWORK_FILE, network, "utf8");
    res.json({ message: "Network updated", network });
  } catch (e) {
    console.error("Error in /api/setNetwork:", e);
    res.status(500).json({ error: e.message });
  }
});

// -------------------------
// Proxy endpoints to backend services
// -------------------------
app.get("/api/servicedetails", async (req, res) => {
  try {
    const resp = await axios.get(BACKEND_URL + "/api/servicedetails");
    res.json(resp.data);
  } catch (e) {
    console.error("Error in /api/servicedetails:", e.response ? e.response.data : e);
    res.status(500).json({ error: e.response ? e.response.data : e.message });
  }
});

app.post("/api/mint", async (req, res) => {
  try {
    const resp = await axios.post(BACKEND_URL + "/api/mint", req.body);
    res.json(resp.data);
  } catch (e) {
    console.error("Error in /api/mint:", e.response ? e.response.data : e);
    res.status(500).json({ error: e.response ? e.response.data : e.message });
  }
});

app.post("/api/confirm", async (req, res) => {
  try {
    const resp = await axios.post(BACKEND_URL + "/api/confirm", req.body);
    res.json(resp.data);
  } catch (e) {
    console.error("Error in /api/confirm:", e.response ? e.response.data : e);
    res.status(500).json({ error: e.response ? e.response.data : e.message });
  }
});

app.get("/api/debug", async (req, res) => {
  try {
    const resp = await axios.get(BACKEND_URL + "/api/debug");
    res.json(resp.data);
  } catch (e) {
    console.error("Error in /api/debug:", e.response ? e.response.data : e);
    res.status(500).json({ error: e.response ? e.response.data : e.message });
  }
});

app.get("/api/console", async (req, res) => {
  try {
    const resp = await axios.get(BACKEND_URL + "/api/console");
    res.setHeader("Content-Type", "text/plain");
    res.send(resp.data);
  } catch (e) {
    console.error("Error in /api/console:", e.response ? e.response.data : e);
    res.status(500).json({ error: e.response ? e.response.data : e.message });
  }
});

app.post("/api/mintNFT", async (req, res) => {
  try {
    const resp = await axios.post(BACKEND_URL + "/api/mintNFT", req.body);
    res.json(resp.data);
  } catch (e) {
    console.error("Error in /api/mintNFT:", e.response ? e.response.data : e);
    res.status(500).json({ error: e.response ? e.response.data : e.message });
  }
});

app.post("/api/xumm/sign", async (req, res) => {
  try {
    const seedData = fs.readFileSync("wallet_secret.txt", "utf8").trim();
    if (!seedData) {
      res.status(404).json({ error: "Wallet secret not found" });
      return;
    }
    const qrcode = require("qrcode");
    const qrCodePNG = await qrcode.toBuffer(seedData, { type: "png", margin: 1, width: 200 });
    const qrCodeB64 = qrCodePNG.toString("base64");
    res.json({
      qrCode: qrCodeB64,
      message: "Scan this QR code with your Xaman app to import the wallet."
    });
  } catch (e) {
    console.error("Error in /api/xumm/sign:", e);
    res.status(500).json({ error: e.message });
  }
});

app.post("/api/clearwallet", async (req, res) => {
  try {
    const clearResponse = await axios.post(BACKEND_URL + "/api/clearwallet", req.body);
    res.json(clearResponse.data);
  } catch (e) {
    console.error("Error in /api/clearwallet:", e.response ? e.response.data : e);
    res.status(500).json({ error: e.response ? e.response.data : e.message });
  }
});

app.post("/api/deriveAddress", async (req, res) => {
  try {
    const { seed } = req.body;
    if (!seed) {
      return res.status(400).json({ error: "Seed not provided" });
    }
    const xrplWallet = require("github.com/xyield/xrpl-go/wallet");
    const wallet = xrplWallet.FromSeed(seed);
    const address = wallet.ClassicAddress();
    res.json({ address });
  } catch (e) {
    console.error("Error in /api/deriveAddress:", e);
    res.status(500).json({ error: e.message });
  }
});

// -------------------------
// New Consensus Endpoint (Trinity)
// -------------------------
app.get("/api/consensus", async (req, res) => {
  try {
    // Fetch consensus details from the local Trinity server running on port 7501
    const consensusResp = await axios.get("http://localhost:7501/consensus");
    res.json(consensusResp.data);
  } catch (e) {
    console.error("Error in /api/consensus:", e.response ? e.response.data : e);
    res.status(500).json({ error: e.response ? e.response.data : e.message });
  }
});

// -------------------------
// New Governance Endpoints
// (These endpoints proxy governance actions to the backend Go server.)
// -------------------------
app.post("/api/governance/propose", async (req, res) => {
  try {
    const resp = await axios.post(BACKEND_URL + "/api/governance/propose", req.body);
    res.json(resp.data);
  } catch (e) {
    console.error("Error in /api/governance/propose:", e.response ? e.response.data : e);
    res.status(500).json({ error: e.response ? e.response.data : e.message });
  }
});

app.post("/api/governance/vote", async (req, res) => {
  try {
    const resp = await axios.post(BACKEND_URL + "/api/governance/vote", req.body);
    res.json(resp.data);
  } catch (e) {
    console.error("Error in /api/governance/vote:", e.response ? e.response.data : e);
    res.status(500).json({ error: e.response ? e.response.data : e.message });
  }
});

app.post("/api/governance/execute", async (req, res) => {
  try {
    const resp = await axios.post(BACKEND_URL + "/api/governance/execute", req.body);
    res.json(resp.data);
  } catch (e) {
    console.error("Error in /api/governance/execute:", e.response ? e.response.data : e);
    res.status(500).json({ error: e.response ? e.response.data : e.message });
  }
});

app.get("/api/governance/whitelist", async (req, res) => {
  try {
    const resp = await axios.get(BACKEND_URL + "/api/governance/whitelist");
    res.json(resp.data);
  } catch (e) {
    console.error("Error in /api/governance/whitelist:", e.response ? e.response.data : e);
    res.status(500).json({ error: e.response ? e.response.data : e.message });
  }
});

app.post("/api/governance/whitelist", async (req, res) => {
  try {
    const resp = await axios.post(BACKEND_URL + "/api/governance/whitelist", req.body);
    res.json(resp.data);
  } catch (e) {
    console.error("Error in POST /api/governance/whitelist:", e.response ? e.response.data : e);
    res.status(500).json({ error: e.response ? e.response.data : e.message });
  }
});

// -------------------------
// Node Status Endpoint
// -------------------------
app.get("/api/nodeStatus", async (req, res) => {
  try {
    const resp = await axios.get(BACKEND_URL + "/api/nodeStatus");
    res.json(resp.data);
  } catch (e) {
    console.error("Error in /api/nodeStatus:", e.response ? e.response.data : e);
    res.status(500).json({ error: e.response ? e.response.data : e.message });
  }
});

// -------------------------
// IPFS content retrieval endpoint
// -------------------------
app.get("/:cid/*?", async (req, res) => {
  const cid = req.params.cid;
  const filePath = req.params[0] || "index.html";
  try {
    const client = create({ url: process.env.IPFS_API || "http://localhost:5001" });
    let chunks = [];
    for await (const file of client.get(`${cid}/${filePath}`)) {
      if (!file.content) continue;
      for await (const chunk of file.content) {
        chunks.push(chunk);
      }
    }
    const fileBuffer = Buffer.concat(chunks);
    if (filePath.endsWith(".html")) {
      res.setHeader("Content-Type", "text/html");
    }
    res.send(fileBuffer);
  } catch (err) {
    console.error("Error retrieving IPFS content:", err);
    res.status(500).send("Error retrieving content from IPFS: " + err.message);
  }
});

// -------------------------
// HTTPS Server setup
// -------------------------
const CERT_DIR = "certs";
const CERT_FILE = CERT_DIR + "/cloudstorm.pem";
const KEY_FILE = CERT_DIR + "/cloudstorm-key.pem";
const credentials = {
  key: fs.readFileSync(KEY_FILE, "utf8"),
  cert: fs.readFileSync(CERT_FILE, "utf8"),
  passphrase: passphrase
};

const PORT = process.env.API_PORT || 3000;
const httpsServer = https.createServer(credentials, app);
httpsServer.listen(PORT, "0.0.0.0", () => {
  console.log("[Node] HTTPS Server running on port " + PORT);
});
