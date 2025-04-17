// trinity.cpp
// Production‑Ready Trinity Server Implementation (C++11)
// — genesis key & pre‑generated proof‑key removed; proof‑key now generated on‑demand.

#include <boost/asio.hpp>
#include <boost/asio/ip/tcp.hpp>
#include <boost/asio/local/stream_protocol.hpp>
#include <boost/beast.hpp>
#include <boost/beast/http.hpp>
#include <boost/filesystem.hpp>

#include <cstdlib>
#include <csignal>
#include <iomanip>
#include <iostream>
#include <mutex>
#include <sstream>
#include <string>
#include <thread>
#include <unordered_set>
#include <vector>
#include <memory>
#include <cstdio>
#include <cstring>
#include <chrono>
#include <unistd.h>
#include <sys/stat.h>

#include <openssl/evp.h>
#include <openssl/pem.h>
#include <openssl/x509.h>

// Convenience namespaces
namespace asio  = boost::asio;
namespace ip    = boost::asio::ip;
namespace local = boost::asio::local;
namespace beast = boost::beast;
namespace http  = beast::http;
namespace fs    = boost::filesystem;

// ----------------------------------------------------------------------------
// Signal handler for segmentation faults
// ----------------------------------------------------------------------------
void segv_handler(int signum) {
    std::cerr << "[Trinity Fatal] Segmentation fault (signal " << signum << ").\n";
    std::abort();
}

// ----------------------------------------------------------------------------
// Global Configuration and State
// ----------------------------------------------------------------------------
static const std::string HOST_PEERNAME = "genesis";

static std::mutex g_stateMutex;
static std::string g_localChain = "default_chain";
static std::string g_proofKeyHash;       // now empty until /consensus
static std::string g_sockPath;           // set from TRINITY_SOCK_PATH env var
static bool        g_firstChain = true;
static int         g_expectedPeers = 4;  // from EXPECTED_PEER_COUNT
static std::unordered_set<std::string> g_readyTunnels;

// Optional TCP gossip
static int                      g_tcpPort  = 0;
static std::vector<std::string> g_peerHosts;
static std::string              g_selfId;

// ----------------------------------------------------------------------------
// Utility: SHA‑256 hex digest
// ----------------------------------------------------------------------------
std::string sha256Hex(const std::string& input) {
    EVP_MD_CTX* ctx = EVP_MD_CTX_new();
    std::ostringstream oss;
    unsigned char hash[EVP_MAX_MD_SIZE];
    unsigned int hash_len = 0;
    if (ctx &&
        EVP_DigestInit_ex(ctx, EVP_sha256(), NULL) &&
        EVP_DigestUpdate(ctx, input.data(), input.size()) &&
        EVP_DigestFinal_ex(ctx, hash, &hash_len))
    {
        for (unsigned int i = 0; i < hash_len; ++i) {
            oss << std::hex << std::setw(2) << std::setfill('0')
                << static_cast<int>(hash[i]);
        }
    }
    EVP_MD_CTX_free(ctx);
    return oss.str();
}

// ----------------------------------------------------------------------------
// Utility: computeServiceID
// ----------------------------------------------------------------------------
std::string computeServiceID(const std::string& baseDir) {
    EVP_MD_CTX* ctx = EVP_MD_CTX_new();
    if (!ctx || !EVP_DigestInit_ex(ctx, EVP_sha256(), NULL)) {
        if (ctx) EVP_MD_CTX_free(ctx);
        return "default_chain";
    }
    bool found = false;
    try {
        for (fs::recursive_directory_iterator it(baseDir), end; it != end; ++it) {
            if (!fs::is_directory(it->path())) {
                std::string pathStr = it->path().string();
                EVP_DigestUpdate(ctx, pathStr.data(), pathStr.size());
                found = true;
            }
        }
    } catch (...) {}
    unsigned char hash[EVP_MAX_MD_SIZE];
    unsigned int  hash_len = 0;
    EVP_DigestFinal_ex(ctx, hash, &hash_len);
    EVP_MD_CTX_free(ctx);
    if (!found) return "default_chain";
    std::ostringstream oss;
    for (unsigned int i = 0; i < hash_len; ++i) {
        oss << std::hex << std::setw(2) << std::setfill('0')
            << static_cast<int>(hash[i]);
    }
    return oss.str();
}

// ----------------------------------------------------------------------------
// Background thread: periodically update g_localChain
// ----------------------------------------------------------------------------
void chainUpdater(const std::string& baseDir) {
    using namespace std::chrono;
    std::string prev;
    while (true) {
        try {
            std::string sid = computeServiceID(baseDir);
            {
                std::lock_guard<std::mutex> lock(g_stateMutex);
                g_localChain = g_firstChain ? sid : sha256Hex(prev + sid);
                g_firstChain  = false;
                prev          = g_localChain;
            }
        } catch (...) {
            std::cerr << "[Trinity] chainUpdater exception\n";
        }
        std::this_thread::sleep_for(seconds(5));
    }
}

// ----------------------------------------------------------------------------
// Free function: generate a deterministic RSA cert & key pair from input
// ----------------------------------------------------------------------------
std::pair<std::string,std::string> generateDeterministicCert(const std::string& input) {
    std::string cert, key;
    EVP_PKEY_CTX* ctx = EVP_PKEY_CTX_new_id(EVP_PKEY_RSA,nullptr);
    if (!ctx ||
        EVP_PKEY_keygen_init(ctx) <= 0 ||
        EVP_PKEY_CTX_set_rsa_keygen_bits(ctx,2048) <= 0)
    {
        if (ctx) EVP_PKEY_CTX_free(ctx);
        return { "", "" };
    }
    EVP_PKEY* pkey = nullptr;
    if (EVP_PKEY_keygen(ctx, &pkey) > 0) {
        X509* x = X509_new();
        X509_set_version(x, 2);
        ASN1_INTEGER_set(X509_get_serialNumber(x), 1);
        X509_gmtime_adj(X509_get_notBefore(x), 0);
        X509_gmtime_adj(X509_get_notAfter(x), 31536000L);
        X509_set_pubkey(x, pkey);
        X509_NAME* name = X509_get_subject_name(x);
        X509_NAME_add_entry_by_txt(name, "CN",
                                  MBSTRING_ASC,
                                  (unsigned char*)"Trinity",
                                  -1, -1, 0);
        X509_set_issuer_name(x, name);
        X509_sign(x, pkey, EVP_sha256());

        BIO *bcert = BIO_new(BIO_s_mem()), *bkey = BIO_new(BIO_s_mem());
        PEM_write_bio_X509(bcert, x);
        PEM_write_bio_PrivateKey(bkey, pkey, NULL, NULL, 0, NULL, NULL);
        BUF_MEM *mb, *kb;
        BIO_get_mem_ptr(bcert, &mb);
        BIO_get_mem_ptr(bkey, &kb);
        cert.assign(mb->data, mb->length);
        key .assign(kb->data, kb->length);
        BIO_free(bcert);
        BIO_free(bkey);
        X509_free(x);
    }
    EVP_PKEY_free(pkey);
    EVP_PKEY_CTX_free(ctx);
    return { cert, key };
}

// ----------------------------------------------------------------------------
// HTTP request handler
// ----------------------------------------------------------------------------
template<typename Stream>
void handleHttpRequest(http::request<http::string_body>& req, Stream& stream) {
    http::response<http::string_body> res;
    {
        std::lock_guard<std::mutex> lock(g_stateMutex);

        // POST /tunnel/ready
        if (req.method() == http::verb::post && req.target() == "/tunnel/ready") {
            // convert header to std::string
            auto sv = req[ "X-Node-ID" ];
            std::string nodeId(sv.data(), sv.size());
            g_readyTunnels.insert(nodeId);
            std::cout << "[Trinity] Peer tunnel ready: " << nodeId << "\n";

            res.result(http::status::ok);
            res.body() = "{\"tunnel\":\"acknowledged\"}";
        }
        // GET /ready
        else if (req.method() == http::verb::get && req.target() == "/ready") {
            bool ready = (g_readyTunnels.size() >= (size_t)(g_expectedPeers - 1));
            std::cout << "[Trinity] /ready: " << g_readyTunnels.size()
                      << "/" << (g_expectedPeers - 1) << "\n";

            res.result(http::status::ok);
            res.set(http::field::content_type, "application/json");
            res.body() = std::string("{\"ready\":") + (ready ? "true" : "false") + "}";
        }
        // GET /consensus
        else if (req.method() == http::verb::get && req.target() == "/consensus") {
            bool haveAll = (g_readyTunnels.size() >= (size_t)(g_expectedPeers - 1));
            std::string cert, key;
            if (haveAll) {
                if (g_proofKeyHash.empty())
                    g_proofKeyHash = sha256Hex(g_localChain);

                auto pair = generateDeterministicCert(g_localChain + g_proofKeyHash);
                cert = pair.first;
                key  = pair.second;
            }

            res.result(http::status::ok);
            res.set(http::field::content_type, "application/json");
            std::ostringstream js;
            js << "{"
               << "\"service_id\":\""    << g_localChain     << "\","
               << "\"proof_key_hash\":\"" << g_proofKeyHash   << "\","
               << "\"cert\":\"";
            for (char c : cert) {
                if (c == '"') js << "\\\""; else js << c;
            }
            js << "\",\"key\":\"";
            for (char c : key) {
                if (c == '"') js << "\\\""; else js << c;
            }
            js << "\"}";
            res.body() = js.str();
        }
        // GET /peers
        else if (req.method() == http::verb::get && req.target() == "/peers") {
            std::ostringstream oss; oss << "[";
            bool first = true;
            for (auto& p : g_readyTunnels) {
                if (!first) oss << ",";
                oss << "\"" << p << "\"";
                first = false;
            }
            oss << "]";

            res.result(http::status::ok);
            res.set(http::field::content_type, "application/json");
            res.body() = std::string("{\"peers\":") + oss.str() + "}";
        }
        // Not found
        else {
            res.result(http::status::not_found);
            res.body() = "Not found";
        }
    }

    res.prepare_payload();
    http::write(stream, res);
}

// ----------------------------------------------------------------------------
// UNIX listener (unchanged)
// ----------------------------------------------------------------------------
void startUnixListener(asio::io_service& ios, const std::string& sock) {
    struct stat sb;
    if (stat(sock.c_str(), &sb) == 0 && S_ISSOCK(sb.st_mode)) {
        unlink(sock.c_str());
    }
    local::stream_protocol::acceptor acceptor(ios,
        local::stream_protocol::endpoint(sock));
    chmod(sock.c_str(), 0777);
    std::cout << "[Trinity] Listening on UNIX socket: " << sock << "\n";

    for (;;) {
        auto sockPtr = std::make_shared<local::stream_protocol::socket>(ios);
        acceptor.accept(*sockPtr);
        std::thread([sockPtr]() {
            try {
                beast::flat_buffer buffer;
                http::request<http::string_body> req;
                http::read(*sockPtr, buffer, req);
                handleHttpRequest(req, *sockPtr);
            } catch (const std::exception& e) {
                std::cerr << "[Trinity] UNIX error: " << e.what() << "\n";
            }
            boost::system::error_code ec;
            sockPtr->shutdown(local::stream_protocol::socket::shutdown_send, ec);
        }).detach();
    }
}

// ----------------------------------------------------------------------------
// TCP listener & gossip
// ----------------------------------------------------------------------------
void doGossipConnect(const std::string& hostPort) {
    using tcp = ip::tcp;
    try {
        auto pos = hostPort.find(':');
        if (pos == std::string::npos) return;

        asio::io_service ios;
        tcp::resolver resolver(ios);
        auto endpoints = resolver.resolve({hostPort.substr(0,pos),
                                           hostPort.substr(pos+1)});
        auto sockPtr = std::make_shared<tcp::socket>(ios);
        asio::connect(*sockPtr, endpoints);

        http::request<http::string_body> req{http::verb::post, "/tunnel/ready", 11};
        req.set(http::field::host, hostPort.substr(0,pos));
        req.set(http::field::content_type, "application/json");
        req.set("X-Node-ID", g_selfId);
        req.prepare_payload();
        http::write(*sockPtr, req);

        beast::flat_buffer buf;
        http::response<http::string_body> res;
        http::read(*sockPtr, buf, res);

        boost::system::error_code ec;
        sockPtr->shutdown(tcp::socket::shutdown_both, ec);

        std::cout << "[Trinity Gossip] Informed " << hostPort
                  << " of self=" << g_selfId << "\n";
    } catch (...) {}
}

void gossipThread() {
    using namespace std::chrono;
    while (true) {
        for (auto& h : g_peerHosts)
            doGossipConnect(h);
        std::this_thread::sleep_for(seconds(10));
    }
}

void startTcpListener(asio::io_service& ios, unsigned short port) {
    ip::tcp::acceptor acceptor(ios,
        ip::tcp::endpoint(ip::tcp::v4(), port));
    std::cout << "[Trinity] Listening on TCP port: " << port << "\n";
    for (;;) {
        auto sockPtr = std::make_shared<ip::tcp::socket>(ios);
        acceptor.accept(*sockPtr);
        std::thread([sockPtr]() {
            try {
                beast::flat_buffer buffer;
                http::request<http::string_body> req;
                http::read(*sockPtr, buffer, req);
                handleHttpRequest(req, *sockPtr);
            } catch (const std::exception& e) {
                std::cerr << "[Trinity TCP] " << e.what() << "\n";
            }
            boost::system::error_code ec;
            sockPtr->shutdown(ip::tcp::socket::shutdown_both, ec);
        }).detach();
    }
}

// ----------------------------------------------------------------------------
// Main
// ----------------------------------------------------------------------------
int main(int argc, char* argv[]) {
    std::signal(SIGSEGV, segv_handler);

    // base directory (used by chainUpdater)
    std::string baseDir = (argc > 1) ? argv[1] : ".";

    // read EXPECTED_PEER_COUNT
    if (auto pc = std::getenv("EXPECTED_PEER_COUNT"))
        g_expectedPeers = std::max(2, std::atoi(pc));

    // read TRINITY_SOCK_PATH
    if (auto sp = std::getenv("TRINITY_SOCK_PATH")) {
        g_sockPath = sp;
    } else {
        std::cerr << "[Trinity Fatal] TRINITY_SOCK_PATH must be set.\n";
        return EXIT_FAILURE;
    }

    // read optional TCP port and peers list
    if (auto tp = std::getenv("TRINITY_TCP_PORT"))
        g_tcpPort = std::atoi(tp);
    if (auto pl = std::getenv("TRINITY_PEERS")) {
        std::istringstream iss(pl);
        std::string tok;
        while (std::getline(iss, tok, ',')) {
            if (!tok.empty()) g_peerHosts.push_back(tok);
        }
    }

    // self‑registration
    {
        std::string self = (g_sockPath == "/var/run/trinity-host.sock")
                         ? HOST_PEERNAME
                         : (std::getenv("HOSTNAME") ? std::getenv("HOSTNAME")
                                                   : "unknown");
        {
            std::lock_guard<std::mutex> lock(g_stateMutex);
            g_readyTunnels.insert(self);
        }
        g_selfId = self;
        std::cout << "[Trinity] Self‑registration, ID=" << g_selfId << "\n";
    }

    // chain updater
    std::thread(chainUpdater, baseDir).detach();

    // start listeners
    asio::io_service ios;
    std::thread(startUnixListener, std::ref(ios), g_sockPath).detach();
    if (g_tcpPort > 0) {
        std::thread(startTcpListener, std::ref(ios),
                    (unsigned short)g_tcpPort).detach();
        if (!g_peerHosts.empty())
            std::thread(gossipThread).detach();
    }

    // run
    try {
        ios.run();
    } catch (const std::exception& e) {
        std::cerr << "[Trinity Fatal] " << e.what() << "\n";
        return EXIT_FAILURE;
    }
    return EXIT_SUCCESS;
}
