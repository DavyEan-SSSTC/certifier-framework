//  Copyright (c) 2021-22, VMware Inc, and the Certifier Authors.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

#include <iostream>

#include "support.h"
#include "certifier.h"
#include "simulated_enclave.h"
#include "application_enclave.h"
#include "cc_helpers.h"

#include <sys/socket.h>
#include <arpa/inet.h>
#include <netinet/in.h>
#include <netdb.h>

#include <openssl/ssl.h>
#include <openssl/rsa.h>
#include <openssl/x509.h>
#include <openssl/evp.h>
#include <openssl/rand.h>
#include <openssl/hmac.h>
#include <openssl/err.h>

#include "gramine_trusted.h"

#include "policy_key.cc"

#define FLAGS_print_all true
static string measurement_file("./binary_trusted_measurements_file.bin");
#define FLAGS_trusted_measurements_file measurement_file
#define FLAGS_read_measurement_file true
#define FLAGS_operation ""
#define FLAGS_client_address "localhost"
#define FLAGS_server_address "localhost"
#define FLAGS_policy_host "localhost"
#define FLAGS_policy_port 8123
#define FLAGS_server_app_host "localhost"
//#define FLAGS_server_app_port 8124
#define FLAGS_server_app_port 39431
static string data_dir = "./server_data/";
#define FLAGS_certificate_file "ca.crt"

#define FLAGS_policy_store_file "store.bin"
#define FLAGS_platform_file_name "platform_file.bin"
#define FLAGS_platform_attest_endorsement "platform_attest_endorsement.bin"
#define FLAGS_attest_key_file "attest_key_file.bin"
#define FLAGS_policy_cert_file "policy_cert_file.bin"
#define FLAGS_measurement_file "example_app.measurement"

static std::string enclave_type;

cc_trust_data* app_trust_data = nullptr;

static bool gramine_initialized = false;
bool test_local_certify(string& enclave_type,
       bool init_from_file, string& file_name,
       string& evidence_descriptor);


bool trust_data_initialized = false;
key_message privatePolicyKey;
key_message publicPolicyKey;
string serializedPolicyCert;
X509* policy_cert= nullptr;

policy_store pStore;
key_message privateAppKey;
key_message publicAppKey;
const int app_symmetric_key_size = 64;
byte app_symmetric_key[app_symmetric_key_size];
key_message symmertic_key_for_protect;
bool connected = false;

void print_trust_data() {
  if (!trust_data_initialized)
    return;
  printf("\nTrust data:\n");
  printf("\nPolicy key\n");
  print_key(publicPolicyKey);
  printf("\nPolicy cert\n");
  print_bytes(serializedPolicyCert.size(), (byte*)serializedPolicyCert.data());
  printf("\n");
  printf("\nPrivate app auth key\n");
  print_key(privateAppKey);
  printf("\nPublic app auth key\n");
  print_key(publicAppKey);
  printf("\nBlob key\n");
  print_key(symmertic_key_for_protect);
  printf("\n\n");
}

bool certifier_test_seal(void) {
  string enclave_type("gramine-enclave");
  string enclave_id("local-machine");

  int secret_to_seal_size = 32;
  byte secret_to_seal[secret_to_seal_size];
  int sealed_size_out = 1024;
  byte sealed[sealed_size_out];
  int recovered_size = 32;
  byte recovered[recovered_size];

  memset(sealed, 0, sealed_size_out);
  memset(recovered, 0, recovered_size);
  for (int i = 0; i < secret_to_seal_size; i++)
    secret_to_seal[i]= (7 * i)%16;

  if (FLAGS_print_all) {
    printf("\nSeal\n");
    printf("to seal  (%d): ", secret_to_seal_size); print_bytes(secret_to_seal_size, secret_to_seal); printf("\n");
  }

  if (!Seal(enclave_type, enclave_id, secret_to_seal_size, secret_to_seal, &sealed_size_out, sealed))
    return false;

  if (FLAGS_print_all) {
    printf("sealed   (%d): ", sealed_size_out); print_bytes(sealed_size_out, sealed); printf("\n");
  }

  if (!Unseal(enclave_type, enclave_id, sealed_size_out, sealed, &recovered_size, recovered))
    return false;

  if (FLAGS_print_all) {
    printf("recovered: (%d)", recovered_size); print_bytes(recovered_size, recovered); printf("\n");
  }

  return true;
}

bool gramine_local_certify() {
  string enclave_type("gramine-enclave");
  string evidence_descriptor("gramine-evidence");
  extern bool simulator_init(void);
  if (!gramine_initialized) {
    if (!simulator_init()) {
      return false;
    }
    gramine_initialized = true;
  }

  if (!test_local_certify(enclave_type,
    FLAGS_read_measurement_file,
    FLAGS_trusted_measurements_file,
    evidence_descriptor)) {
    printf("test_local_certify failed\n");
    return false;
  }

  gramine_initialized = false;
  return true;
}

bool gramine_seal() {
  if (!certifier_test_seal()) {
    printf("Sealing test failed\n");
    return false;
  }
  printf("Sealing test succeeded\n");
  return true;
}

bool gramine_setup_certifier_functions(GramineCertifierFunctions gramineFuncs) {
  setFuncs(gramineFuncs);
  return true;
}

string pem_cert_chain;

bool certifier_init(char* usr_data_dir, size_t usr_data_dir_size) {
  static const char rnd_seed[] =
    "string to make the random number generator think it has entropy";

  RAND_seed(rnd_seed, sizeof rnd_seed);
  std::string usr_data = usr_data_dir;
  data_dir =  usr_data + "/";
  printf("Using data_dir: %s\n", data_dir.c_str());

  if (gramine_initialized) {
    return true;
  }

  SSL_library_init();
  printf("Done SSL init\n");

  string enclave_type("gramine-enclave");
  string purpose("authentication");

  string store_file(data_dir);
  store_file.append(FLAGS_policy_store_file);

  app_trust_data = new cc_trust_data(enclave_type, purpose, store_file);
  if (app_trust_data == nullptr) {
    printf("couldn't initialize trust object\n");
    return 1;
  }

  // Init policy key info
  if (!app_trust_data->init_policy_key(initialized_cert_size,
                                       initialized_cert)) {
    printf("Can't init policy key\n");
    return false;
  }

  string cert(data_dir);
  cert.append(FLAGS_certificate_file);
  if (!app_trust_data->initialize_gramine_enclave_data(cert)) {
      printf("Can't init Gramine enclave\n");
      return false;
  }

  gramine_initialized = true;

  return true;
}

bool cold_init() {
  // Standard algorithms for the enclave
  string public_key_alg("rsa-2048");
  string symmetric_key_alg("aes-256");;
  string hash_alg("sha-256");
  string hmac_alg("sha-256-hmac");

  if (!app_trust_data->cold_init(public_key_alg, symmetric_key_alg,
        hash_alg, hmac_alg)) {
    printf("cold-init failed\n");
    return false;
  }

  return true;
}

bool warm_restart() {
  if (!app_trust_data->warm_restart()) {
      printf("warm_restart failed\n");
      return false;
    }

  return true;
}

bool certify_me() {
  printf("Begin certify_me\n");
  if (!app_trust_data->certify_me(FLAGS_policy_host, FLAGS_policy_port)) {
      printf("certify_me failed\n");
      return false;
    }
  return true;
}

void server_application(secure_authenticated_channel& channel) {

  printf("Server peer id is %s\n", channel.peer_id_.c_str());
  if (channel.peer_cert_ != nullptr) {
    printf("Server peer cert is:\n");
#ifdef DEBUG
    X509_print_fp(stdout, channel.peer_cert_);
#endif
  }

  // Read message from client over authenticated, encrypted channel
  string out;
  int n = channel.read(&out);
  printf("SSL server read: %s\n", (const char*) out.data());

  // Reply over authenticated, encrypted channel
  const char* msg = "Hi from your secret server\n";
  channel.write(strlen(msg), (byte*)msg);
  connected = true;
}

void gramine_server_dispatch(const string& host_name, int port,
      string& asn1_root_cert, key_message& private_key,
      const string& private_key_cert, void (*func)(secure_authenticated_channel&)) {

  SSL_load_error_strings();

  //printf("gramine_server_dispatch begin...\n");
  X509* root_cert = X509_new();
  if (!asn1_to_x509(asn1_root_cert, root_cert)) {
    printf("Can't convert cert\n");
    return;
  }

  // Get a socket.
  int sock = -1;
  if (!open_server_socket(host_name, port, &sock)) {
    printf("Can't open server socket\n");
    return;
  }
  //printf("gramine_server_dispatch done sock...\n");

  // Set up TLS handshake data.
  SSL_METHOD* method = (SSL_METHOD*) TLS_server_method();
  SSL_CTX* ctx = SSL_CTX_new(method);
  if (ctx == NULL) {
    printf("SSL_CTX_new failed (1)\n");
    return;
  }
  X509_STORE* cs = SSL_CTX_get_cert_store(ctx);
  X509_STORE_add_cert(cs, root_cert);

#if 1
  X509* x509_auth_cert = X509_new();
  if (asn1_to_x509(private_key_cert, x509_auth_cert)) {
    X509_STORE_add_cert(cs, x509_auth_cert);
  }
#endif

  if (!load_server_certs_and_key(root_cert, private_key, ctx)) {
    printf("SSL_CTX_new failed (2)\n");
    return;
  }

  printf("gramine_server_dispatch done load_server certs...\n");

  const long flags = SSL_OP_NO_SSLv2 | SSL_OP_NO_SSLv3 | SSL_OP_NO_COMPRESSION;
  SSL_CTX_set_options(ctx, flags);

  printf("gramine_server_dispatch done setopts...\n");
  // Verify peer
  SSL_CTX_set_verify(ctx, SSL_VERIFY_PEER | SSL_VERIFY_FAIL_IF_NO_PEER_CERT, nullptr);
  // For debug: SSL_CTX_set_verify(ctx, SSL_VERIFY_PEER, verify_callback);

  printf("Done SSL CTX, going to listen loop\n");
  fflush(stdout);
  while (1) {
    //if (connected) {
    //  break;
    //}
#ifdef DEBUG
    printf("at accept\n");
#endif
    printf("at accept\n");
  fflush(stdout);
    struct sockaddr_in addr;
    unsigned int len = sizeof(sockaddr_in);
    int client = accept(sock, (struct sockaddr*)&addr, &len);
    string my_role("server");
    secure_authenticated_channel nc(my_role);
    if (!nc.init_server_ssl(host_name, port, asn1_root_cert, private_key, private_key_cert)) {
      continue;
    }
    nc.ssl_ = SSL_new(ctx);
    SSL_set_fd(nc.ssl_, client);
    nc.sock_ = client;
    nc.server_channel_accept_and_auth(func);
  }
}

bool setup_server_ssl() {
  bool ret = true;

  if (!app_trust_data->warm_restart()) {
    printf("warm-restart failed\n");
    ret = false;
    goto done;
  }

  printf("running as server\n");

  //TODO: REMOVE
  //server_dispatch(FLAGS_server_app_host, FLAGS_server_app_port,
  gramine_server_dispatch(FLAGS_server_app_host, FLAGS_server_app_port,
      app_trust_data->serialized_policy_cert_,
      app_trust_data->private_auth_key_,
      app_trust_data->private_auth_key_.certificate(),
      server_application);

done:
  // app_trust_data->print_trust_data();
  app_trust_data->clear_sensitive_data();
  return ret;
}

void client_application(secure_authenticated_channel& channel) {

  printf("Client peer id is %s\n", channel.peer_id_.c_str());
  if (channel.peer_cert_ != nullptr) {
    printf("Client peer cert is:\n");
#ifdef DEBUG
    X509_print_fp(stdout, channel.peer_cert_);
#endif
  }

  // client sends a message over authenticated, encrypted channel
  const char* msg = "Hi from your secret client\n";
  channel.write(strlen(msg), (byte*)msg);

  // Get server response over authenticated, encrypted channel and print it
  string out;
  int n = channel.read(&out);
  printf("SSL client read: %s\n", out.data());
}

bool setup_client_ssl() {
  bool ret = true;
  string my_role("client");
  secure_authenticated_channel channel(my_role);

  if (!app_trust_data->warm_restart()) {
    printf("warm-restart failed\n");
    ret = false;
    goto done;
  }

  printf("running as client\n");
  if (!app_trust_data->cc_auth_key_initialized_ ||
      !app_trust_data->cc_policy_info_initialized_) {
    printf("trust data not initialized\n");
    ret = 1;
    goto done;
  }

  if (!channel.init_client_ssl(FLAGS_server_app_host, FLAGS_server_app_port,
        app_trust_data->serialized_policy_cert_,
        app_trust_data->private_auth_key_,
        app_trust_data->private_auth_key_.certificate())) {
    printf("Can't init client app\n");
    ret = 1;
    goto done;
  }

  // This is the actual application code.
  client_application(channel);

done:
  // app_trust_data->print_trust_data();
  app_trust_data->clear_sensitive_data();
  return ret;
}
