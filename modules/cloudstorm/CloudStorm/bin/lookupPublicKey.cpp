#pragma once
#include <iostream>
#include <string>
#include <stdexcept>
#include <chrono>
#include <curl/curl.h>
#include <nlohmann/json.hpp>
namespace xrpl_lookup {
using json = nlohmann::json;
struct BenchmarkResult {
    std::string pubKey;
    double payloadCreation;
    double curlInit;
    double curlPerform;
    double jsonParse;
    double keyExtraction;
    double total;
};
inline size_t WriteCallback(void* contents, size_t size, size_t nmemb, void* userp) {
    std::string* s = static_cast<std::string*>(userp);
    size_t totalSize = size * nmemb;
    s->append(static_cast<char*>(contents), totalSize);
    return totalSize;
}
inline BenchmarkResult lookupPublicKeyBenchmark(const std::string &address) {
    BenchmarkResult result;
    auto t_start = std::chrono::high_resolution_clock::now();
    auto t_payload_start = std::chrono::high_resolution_clock::now();
    json reqPayload = {
        {"method", "account_tx"},
        {"params", {{
            {"account", address},
            {"ledger_index_min", -1},
            {"ledger_index_max", -1},
            {"limit", 10},
            {"binary", false}
        }}},
        {"id", 1}
    };
    std::string requestData = reqPayload.dump();
    auto t_payload_end = std::chrono::high_resolution_clock::now();
    result.payloadCreation = std::chrono::duration<double>(t_payload_end - t_payload_start).count();
    auto t_curl_init_start = std::chrono::high_resolution_clock::now();
    CURL* curl = curl_easy_init();
    if(!curl) { throw std::runtime_error("CURL initialization failed"); }
    struct curl_slist* headers = curl_slist_append(nullptr, "Content-Type: application/json");
    curl_easy_setopt(curl, CURLOPT_URL, "https://s2.ripple.com:51234/");
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);
    curl_easy_setopt(curl, CURLOPT_POSTFIELDS, requestData.c_str());
    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, WriteCallback);
    std::string responseData;
    curl_easy_setopt(curl, CURLOPT_WRITEDATA, &responseData);
    auto t_curl_init_end = std::chrono::high_resolution_clock::now();
    result.curlInit = std::chrono::duration<double>(t_curl_init_end - t_curl_init_start).count();
    auto t_curl_perform_start = std::chrono::high_resolution_clock::now();
    CURLcode res = curl_easy_perform(curl);
    auto t_curl_perform_end = std::chrono::high_resolution_clock::now();
    result.curlPerform = std::chrono::duration<double>(t_curl_perform_end - t_curl_perform_start).count();
    if(res != CURLE_OK) {
        std::string errorMsg = curl_easy_strerror(res);
        curl_easy_cleanup(curl);
        curl_slist_free_all(headers);
        throw std::runtime_error("curl_easy_perform() failed: " + errorMsg);
    }
    curl_easy_cleanup(curl);
    curl_slist_free_all(headers);
    auto t_json_parse_start = std::chrono::high_resolution_clock::now();
    json responseJson;
    try {
        responseJson = json::parse(responseData);
    } catch(const json::parse_error &e) {
        throw std::runtime_error("JSON parse error: " + std::string(e.what()) + " Raw response: " + responseData);
    }
    auto t_json_parse_end = std::chrono::high_resolution_clock::now();
    result.jsonParse = std::chrono::duration<double>(t_json_parse_end - t_json_parse_start).count();
    if(!responseJson.contains("result") || !responseJson["result"].contains("transactions")) {
        throw std::runtime_error("Unexpected response format: " + responseData);
    }
    auto t_key_ext_start = std::chrono::high_resolution_clock::now();
    for(const auto &txEntry : responseJson["result"]["transactions"]) {
        if(txEntry.contains("tx") && txEntry["tx"].contains("SigningPubKey")) {
            std::string pubKey = txEntry["tx"]["SigningPubKey"];
            if(!pubKey.empty() && pubKey != "0") {
                result.pubKey = pubKey;
                break;
            }
        }
    }
    auto t_key_ext_end = std::chrono::high_resolution_clock::now();
    result.keyExtraction = std::chrono::duration<double>(t_key_ext_end - t_key_ext_start).count();
    auto t_end = std::chrono::high_resolution_clock::now();
    result.total = std::chrono::duration<double>(t_end - t_start).count();
    if(result.pubKey.empty()) {
        throw std::runtime_error("No public key found for " + address);
    }
    return result;
}
} 
int main(int argc, char** argv) {
    if(argc < 2) {
        std::cerr << "Usage: lookupPublicKey <address>\n";
        return 1;
    }
    std::string address = argv[1];
    try {
        auto result = xrpl_lookup::lookupPublicKeyBenchmark(address);
        std::cout << result.pubKey;
    } catch(const std::exception &e) {
        std::cerr << "Error: " << e.what();
        return 1;
    }
    return 0;
}
