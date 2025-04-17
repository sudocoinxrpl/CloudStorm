import fs from 'fs';
import https from 'https';
import express from 'express';
import cors from 'cors';
import axios from 'axios';
import helmet from 'helmet';
import morgan from 'morgan';
import path from 'path';
import { create } from 'ipfs-http-client';
import qrcode from 'qrcode';

const app = express();
const NETWORK_FILE = "network.txt";
const BACKEND_URL = process.env.BACKEND_URL || "http://localhost:5115";
const TRINITY_CONSENSUS_URL = process.env.TRINITY_URL || "http://localhost:7501/consensus";

app.use(helmet());
app.use(cors());
app.use(express.json());
app.use(morgan("combined"));

app.use((req, res, next) => {
  const secure = req.secure || req.headers["x-forwarded-proto"] === "https";
  const isLocal = ["localhost", "127.0.0.1"].includes(req.hostname);
  if (secure || isLocal) {
    res.setHeader("Cross-Origin-Opener-Policy", "same-origin");
  }
  res.setHeader(
    "Content-Security-Policy",
    "default-src 'self' 'unsafe-inline' https://unpkg.com; img-src 'self' data: https://unpkg.com"
  );
  next();
});

app.use(express.static(path.join(path.resolve(), "public"), { index: "info.html" }));

// API: Network endpoints
app.get("/api/getNetwork", (req, res) => {
  try {
    const network = fs.existsSync(NETWORK_FILE)
      ? fs.readFileSync(NETWORK_FILE, "utf8").trim()
      : "mainnet";
    res.json({ network });
  } catch (e) {
    console.error(e);
    res.status(500).json({ error: e.message });
  }
});

app.post("/api/setNetwork", (req, res) => {
  try {
    const { network } = req.body;
    if (!network) return res.status(400).json({ error: "Network not provided" });
    fs.writeFileSync(NETWORK_FILE, network, "utf8");
    res.json({ message: "Network updated", network });
  } catch (e) {
    console.error(e);
    res.status(500).json({ error: e.message });
  }
});

// Proxy backend endpoints
const proxyEndpoints = [
  "/api/servicedetails", "/api/mint", "/api/confirm", "/api/debug", "/api/console",
  "/api/mintNFT", "/api/clearwallet", "/api/governance/propose",
  "/api/governance/vote", "/api/governance/execute", "/api/governance/whitelist",
  "/api/nodeStatus"
];

proxyEndpoints.forEach(endpoint => {
  app.all(endpoint, async (req, res) => {
    try {
      const method = req.method.toLowerCase();
      const resp = await axios({
        method,
        url: `${BACKEND_URL}${endpoint}`,
        data: req.body
      });
      res.json(resp.data);
    } catch (e) {
      const errorData = e.response?.data || e.message;
      console.error(`Error in ${endpoint}:`, errorData);
      res.status(500).json({ error: errorData });
    }
  });
});

// XUMM wallet QR code endpoint
app.post("/api/xumm/sign", async (req, res) => {
  try {
    const seedData = fs.readFileSync("wallet_secret.txt", "utf8").trim();
    if (!seedData) return res.status(404).json({ error: "Wallet secret not found" });
    const qrCodePNG = await qrcode.toBuffer(seedData, { type: "png", margin: 1, width: 200 });
    res.json({
      qrCode: qrCodePNG.toString("base64"),
      message: "Scan QR code with Xaman app to import wallet."
    });
  } catch (e) {
    console.error(e);
    res.status(500).json({ error: e.message });
  }
});

// Trinity consensus endpoint (pass-through)
app.get("/api/consensus", async (req, res) => {
  try {
    const resp = await axios.get(TRINITY_CONSENSUS_URL);
    res.json(resp.data);
  } catch (e) {
    console.error(e);
    res.status(500).json({ error: e.response?.data || e.message });
  }
});

// IPFS content retrieval endpoint
app.get("/:cid/*?", async (req, res) => {
  const cid = req.params.cid;
  const filePath = req.params[0] || "index.html";
  const client = create({ url: process.env.IPFS_API || "http://localhost:5001" });
  try {
    const chunks = [];
    for await (const file of client.cat(`${cid}/${filePath}`)) {
      chunks.push(file);
    }
    const fileBuffer = Buffer.concat(chunks);
    const contentType = filePath.endsWith(".html") ? "text/html" : "application/octet-stream";
    res.setHeader("Content-Type", contentType);
    res.send(fileBuffer);
  } catch (err) {
    console.error(err);
    res.status(500).send(`IPFS Error: ${err.message}`);
  }
});

// Fetch SSL certificates from Trinity and start HTTPS server
async function startServer() {
  try {
    const { data } = await axios.get(TRINITY_CONSENSUS_URL);

    if (!data.cert || !data.key) {
      throw new Error("Trinity did not return valid SSL certificates.");
    }

    const credentials = {
      key: data.key,
      cert: data.cert
    };

    const PORT = process.env.API_PORT || 3000;
    https.createServer(credentials, app).listen(PORT, "0.0.0.0", () => {
      console.log(`[Node] HTTPS server running securely on port ${PORT}`);
    });
  } catch (err) {
    console.error("[FATAL] Unable to obtain SSL certificates from Trinity:", err.message);
    process.exit(1);
  }
}

// Explicit startup
startServer();
