// trinity.cpp
// This container server maintains a local chain computed from a base directory,
// calculates a container proof key hash from the environment variable CONTAINER_PROOF_KEY,
// and exposes an HTTP endpoint (/consensus) returning {"service_id", "proof_key_hash"}.
// It also exposes a WebSocket endpoint (/ws) for gossiping chain updates with peers,
// and allows dynamic peer addition via /addPeer?host=xxx&port=7501.
// Automatic local node discovery (e.g., via Docker API) is not implemented here.
// Compile with:
//   g++ -std=c++17 trinity.cpp -o trinity -lboost_system -lssl -lcrypto -lpthread

#include <boost/asio.hpp>
#include <boost/asio/connect.hpp>
#include <boost/beast.hpp>
#include <boost/beast/websocket.hpp>
#include <boost/beast/http.hpp>
#include <filesystem>
#include <iostream>
#include <list>
#include <mutex>
#include <thread>
#include <chrono>
#include <cstdlib>
#include <string>
#include <sstream>
#include <unordered_set>
#include <iomanip>
#include <openssl/evp.h>

namespace asio  = boost::asio;
namespace beast = boost::beast;
namespace http  = beast::http;
namespace websocket = beast::websocket;
namespace fs    = std::filesystem;
using tcp = boost::asio::ip::tcp;
bool g_oneshotMode = false;

// Compute SHA-256 using non-deprecated OpenSSL EVP interface.
static std::string sha256Hex(const std::string &input) {
    EVP_MD_CTX *mdctx = EVP_MD_CTX_new();
    if (!mdctx) {
        std::cerr << "EVP_MD_CTX_new failed" << std::endl;
        return "";
    }
    unsigned char hash[EVP_MAX_MD_SIZE];
    unsigned int hash_len = 0;
    if (EVP_DigestInit_ex(mdctx, EVP_sha256(), nullptr) != 1) {
        EVP_MD_CTX_free(mdctx);
        return "";
    }
    if (EVP_DigestUpdate(mdctx, input.data(), input.size()) != 1) {
        EVP_MD_CTX_free(mdctx);
        return "";
    }
    if (EVP_DigestFinal_ex(mdctx, hash, &hash_len) != 1) {
        EVP_MD_CTX_free(mdctx);
        return "";
    }
    EVP_MD_CTX_free(mdctx);
    std::ostringstream oss;
    for (unsigned int i = 0; i < hash_len; i++)
        oss << std::hex << std::setw(2) << std::setfill('0') << (int)hash[i];
    return oss.str();
}

// Compute a service ID by hashing all file paths in baseDir.
// If no files are found, returns a default string.
static std::string computeServiceID(const std::string &baseDir) {
    EVP_MD_CTX *mdctx = EVP_MD_CTX_new();
    if (!mdctx) {
        std::cerr << "EVP_MD_CTX_new failed" << std::endl;
        return "default_chain";
    }
    if (EVP_DigestInit_ex(mdctx, EVP_sha256(), nullptr) != 1) {
        EVP_MD_CTX_free(mdctx);
        return "default_chain";
    }
    bool fileFound = false;
    for (auto &p : fs::recursive_directory_iterator(baseDir, fs::directory_options::skip_permission_denied)) {
        if (!fs::is_directory(p.path())) {
            fileFound = true;
            auto pathStr = p.path().string();
            if (EVP_DigestUpdate(mdctx, pathStr.data(), pathStr.size()) != 1) {
                EVP_MD_CTX_free(mdctx);
                return "default_chain";
            }
        }
    }
    unsigned char hash[EVP_MAX_MD_SIZE];
    unsigned int hash_len = 0;
    if (EVP_DigestFinal_ex(mdctx, hash, &hash_len) != 1) {
        EVP_MD_CTX_free(mdctx);
        return "default_chain";
    }
    EVP_MD_CTX_free(mdctx);
    std::ostringstream oss;
    if (!fileFound) {
        oss << "default_chain";
    } else {
        for (unsigned int i = 0; i < hash_len; i++)
            oss << std::hex << std::setw(2) << std::setfill('0') << (int)hash[i];
    }
    return oss.str();
}

// Global container state.
static std::mutex g_stateMutex;
static std::string g_localChain;
static std::string g_proofKeyHash;
static bool g_firstChain = true;

// Peer management.
static std::mutex g_peersMutex;
static std::unordered_set<std::string> g_peerSet; // stores "host:port"
static std::list<std::weak_ptr<websocket::stream<tcp::socket>>> g_wsPeers;

// Periodically update the local chain and broadcast it to connected WS peers.
static void chainUpdater(const std::string &baseDir) {
    using namespace std::chrono_literals;
    std::string prev;
    while(true) {
        try {
            auto sid = computeServiceID(baseDir);
            std::string newChain;
            {
                std::lock_guard<std::mutex> lk(g_stateMutex);
                if(g_firstChain) {
                    newChain = sid;
                    g_firstChain = false;
                } else {
                    newChain = sha256Hex(prev + sid);
                }
                g_localChain = newChain;
                prev = newChain;
            }
            std::cout << "[Trinity] new local chain: " << newChain << std::endl;
            // Broadcast new chain to all connected WS peers.
            {
                std::lock_guard<std::mutex> lk(g_peersMutex);
                for(auto &wref : g_wsPeers) {
                    if(auto wptr = wref.lock()) {
                        beast::error_code ec;
                        wptr->text(true);
                        wptr->write(asio::buffer(newChain), ec);
                    }
                }
            }
        } catch(std::exception &ex) {
            std::cerr << "[chainUpdater] error: " << ex.what() << std::endl;
        }
        std::this_thread::sleep_for(5s);
    }
}

// Attempt to connect to a known peer via WebSocket.
static void connectToPeer(asio::io_context &ioc, const std::string &host, const std::string &port) {
    try {
        tcp::resolver resolver(ioc);
        auto results = resolver.resolve(host, port);
        if (results.begin() == results.end()) {
            std::cerr << "[connectToPeer] no endpoints found for " << host << ":" << port << std::endl;
            return;
        }
        auto ws = std::make_shared<websocket::stream<tcp::socket>>(tcp::socket(ioc));
        boost::asio::connect(ws->next_layer(), results);
        ws->handshake(host, "/ws");
        {
            std::lock_guard<std::mutex> lk(g_peersMutex);
            g_wsPeers.push_back(ws);
        }
        // Read loop.
        auto doRead = std::make_shared<std::function<void()>>();
        *doRead = [ws, doRead]() {
            auto buf = std::make_shared<beast::flat_buffer>();
            ws->async_read(*buf, [ws, buf, doRead](beast::error_code ec, std::size_t) {
                if(ec) {
                    std::cerr << "[connectToPeer] peer disconnected: " << ec.message() << std::endl;
                    return;
                }
                auto msg = beast::buffers_to_string(buf->data());
                std::cout << "[connectToPeer] received from peer: " << msg << std::endl;
                (*doRead)();
            });
        };
        (*doRead)();
    } catch(std::exception &ex) {
        std::cerr << "[connectToPeer] " << host << ":" << port << " failed: " << ex.what() << std::endl;
    }
}

// Serve an HTTP request.
template<class Body, class Allocator>
void handleHttpRequest(
    http::request<Body, http::basic_fields<Allocator>> &&req,
    beast::tcp_stream &stream)
{
    std::cout << "[Trinity] HTTP request for target: " << req.target() << std::endl;
    if(req.method() != http::verb::get) {
        http::response<http::string_body> bad{ http::status::bad_request, req.version() };
        bad.set(http::field::server, "Trinity/1.0");
        bad.set(http::field::content_type, "text/plain");
        bad.body() = "Only GET is supported.\n";
        bad.prepare_payload();
        http::write(stream, bad);
        return;
    }
    if(req.target() == "/consensus") {
        std::string chain, pk;
        {
            std::lock_guard<std::mutex> lk(g_stateMutex);
            chain = g_localChain;
            pk = g_proofKeyHash;
        }
        std::ostringstream oss;
        oss << "{ \"service_id\": \"" << chain << "\", \"proof_key_hash\": \"" << pk << "\" }";
        http::response<http::string_body> res{ http::status::ok, req.version() };
        res.set(http::field::server, "Trinity/1.0");
        res.set(http::field::content_type, "application/json");
        res.body() = oss.str();
        res.prepare_payload();
        http::write(stream, res);
        return;
    }
    if(req.target().starts_with("/addPeer?")) {
        // Expect query: /addPeer?host=xxx&port=7501
        std::string query(req.target().begin()+8, req.target().end());
        std::string host, port;
        std::istringstream iss(query);
        std::string token;
        while(std::getline(iss, token, '&')) {
            auto eqPos = token.find('=');
            if(eqPos != std::string::npos) {
                auto key = token.substr(0, eqPos);
                auto val = token.substr(eqPos+1);
                if(key == "host") host = val;
                else if(key == "port") port = val;
            }
        }
        if(!host.empty() && !port.empty()) {
            std::string combined = host + ":" + port;
            {
                std::lock_guard<std::mutex> lk(g_peersMutex);
                g_peerSet.insert(combined);
            }
            http::response<http::string_body> res{ http::status::ok, req.version() };
            res.set(http::field::server, "Trinity/1.0");
            res.set(http::field::content_type, "text/plain");
            res.body() = "Peer added.\n";
            res.prepare_payload();
            http::write(stream, res);
        } else {
            http::response<http::string_body> bad{ http::status::bad_request, req.version() };
            bad.set(http::field::server, "Trinity/1.0");
            bad.set(http::field::content_type, "text/plain");
            bad.body() = "Missing host or port.\n";
            bad.prepare_payload();
            http::write(stream, bad);
        }
        return;
    }
    // Not found.
    http::response<http::string_body> notFound{ http::status::not_found, req.version() };
    notFound.set(http::field::server, "Trinity/1.0");
    notFound.set(http::field::content_type, "text/plain");
    notFound.body() = "Not Found.\n";
    notFound.prepare_payload();
    http::write(stream, notFound);
}

// Upgrade an HTTP connection to WebSocket if target == /ws.
static void doWebsocket(
    beast::tcp_stream &&stream,
    http::request<http::string_body> req)
{
    tcp::socket sock = stream.release_socket();
    auto ws = std::make_shared<websocket::stream<tcp::socket>>(std::move(sock));
    ws->async_accept(req, [ws](beast::error_code ec) {
        if(ec) {
            std::cerr << "WebSocket accept error: " << ec.message() << std::endl;
            return;
        }
        {
            std::lock_guard<std::mutex> lk(g_peersMutex);
            g_wsPeers.push_back(ws);
        }
        auto doRead = std::make_shared<std::function<void()>>();
        *doRead = [ws, doRead]() {
            auto buffer = std::make_shared<beast::flat_buffer>();
            ws->async_read(*buffer, [ws, buffer, doRead](beast::error_code ec, std::size_t) {
                if(ec) {
                    return;
                }
                auto msg = beast::buffers_to_string(buffer->data());
                std::cout << "[Trinity] WS message from peer: " << msg << std::endl;
                (*doRead)();
            });
        };
        (*doRead)();
    });
}

// Accept incoming connections on the specified port.
static void listener(asio::io_context &ioc, unsigned short port) {
    tcp::acceptor acceptor(ioc, tcp::endpoint(asio::ip::make_address("0.0.0.0"), port));
    while(true) {
        beast::error_code ec;
        tcp::socket socket(ioc);
        acceptor.accept(socket, ec);
        if(ec) {
            std::cerr << "Accept error: " << ec.message() << std::endl;
            continue;
        }
        std::thread([sock = std::move(socket)]() mutable {
            beast::tcp_stream stream(std::move(sock));
            beast::flat_buffer buffer;
            http::request<http::string_body> req;
            beast::error_code ec;
            http::read(stream, buffer, req, ec);
            if(ec) {
                if(ec != http::error::end_of_stream)
                    std::cerr << "HTTP read error: " << ec.message() << std::endl;
                return;
            }
            if(req.target() == "/ws") {
                doWebsocket(std::move(stream), std::move(req));
            } else {
                handleHttpRequest(std::move(req), stream);
            }
            beast::error_code ignored;
            stream.socket().shutdown(asio::ip::tcp::socket::shutdown_send, ignored);
        }).detach();
    }
}

int main(int argc, char* argv[])
{
    std::string baseDir = (argc > 1) ? argv[1] : ".";
    // Remove multi-port support: always use port 7501.
    unsigned short port = 7501;
    const char* pkEnv = std::getenv("CONTAINER_PROOF_KEY");
    std::string containerProofKey = pkEnv ? pkEnv : "default_container_proof_key";
    g_proofKeyHash = sha256Hex(containerProofKey);
    std::cout << "[Trinity] containerProofKeyHash: " << g_proofKeyHash << std::endl;

    // Start chain updater thread.
    std::thread updater(chainUpdater, baseDir);
    updater.detach();

    try {
        asio::io_context ioc;
        std::thread listenerThread([&ioc, port]() {
            listener(ioc, port);
        });
        listenerThread.detach();

        // Periodically attempt to connect to known peers.
        std::thread([&ioc](){
            using namespace std::chrono_literals;
            while(true) {
                {
                    std::lock_guard<std::mutex> lk(g_peersMutex);
                    for(const auto &p : g_peerSet) {
                        auto delim = p.find(':');
                        if(delim != std::string::npos) {
                            auto host = p.substr(0, delim);
                            auto port = p.substr(delim+1);
                            connectToPeer(ioc, host, port);
                        }
                    }
                }
                std::this_thread::sleep_for(15s);
            }
        }).detach();

        std::cout << "[Trinity] listening on port " << port << "...\n";
        ioc.run();
    } catch(std::exception &ex) {
        std::cerr << "[Trinity] fatal error: " << ex.what() << std::endl;
        return 1;
    }
    return 0;
}
