#include <openssl/ssl.h>
#include <openssl/rsa.h>
#include <openssl/x509.h>
#include <openssl/evp.h>
#include <openssl/rand.h>
#include <openssl/hmac.h>
#include <openssl/x509v3.h>

#include "support.h" 
#include "simulated_enclave.h" 
#include "application_enclave.h" 
#include "certifier.pb.h" 

#include <string>
using std::string;

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

bool initialized = false;
int reader = 0;
int writer = 0;

bool application_Init(int read_fd, int write_fd) {
  reader = read_fd;
  writer = write_fd;
  initialized = true;
  return true;
}

bool application_GetCerts(int* size_out, byte* out) {
  app_request req;
  app_response rsp;

  req.set_function("getcerts");
  string str_req;
  req.SerializeToString(&str_req);
  write(writer, (byte*)str_req.data(), str_req.size());
  int t_size = 4096;
  byte t_out[t_size];
  int n= read(reader, t_out, t_size);
  if (n < 0)
    return false;
  string r_str;
  r_str.assign((char*)t_out, n);
  if (!rsp.ParseFromString(r_str))
    return false;
  if (rsp.function() != "getcerts" || rsp.status() != "succeeded")
    return false;
  if (*size_out > rsp.args(0).size())
    return false;
  *size_out = rsp.args(0).size();
  memcpy(out, rsp.args(0).data(), *size_out);
  return true;
}

bool application_Seal(int in_size, byte* in, int* size_out, byte* out) {
  app_request req;
  app_response rsp;

  req.set_function("seal");

  // send request
  string req_arg_str;
  req_arg_str.assign((char*)in, in_size);
  req.add_args(req_arg_str);
  string req_str;
  req.SerializeToString(&req_str);
  write(writer, (byte*)req_str.data(), req_str.size());

  // get response
  int t_size = in_size + 200; // fix
  byte t_out[t_size];
  int n = read(reader, t_out, t_size);
  if (n < 0)
    return false;
  string rsp_str;
  rsp_str.assign((char*)t_out, n);
  if (!rsp.ParseFromString(rsp_str))
    return false;
  if (rsp.function() != "seal" || rsp.status() != "succeeded") {
printf("application_Seal, function: %s, status: %s\n", rsp.function().c_str(), rsp.status().c_str());
    return false;
  }

  if (*size_out < rsp.args(0).size()) {
printf("application_Seal, output too big\n");
    return false;
  }
  *size_out = rsp.args(0).size();
  memcpy(out, rsp.args(0).data(), *size_out);
  return true;
}

bool application_Unseal(int in_size, byte* in, int* size_out, byte* out) {
  app_request req;
  app_response rsp;

  // request
  req.set_function("unseal");

  string req_arg_str;
  req_arg_str.assign((char*)in, in_size);
  req.add_args(req_arg_str);

  string req_str;
  req.SerializeToString(&req_str);
  write(writer, (byte*)req_str.data(), req_str.size());
  int t_size = in_size + 200; // Fix
  byte t_out[t_size];
  int n= read(reader, t_out, t_size);
  if (n < 0)
    return false;

  string rsp_str;
  rsp_str.assign((char*)t_out, n);
  if (!rsp.ParseFromString(rsp_str))
    return false;
  if (rsp.function() != "unseal" || rsp.status() != "succeeded") {
printf("application_Unseal, function: %s, status: %s\n", rsp.function().c_str(), rsp.status().c_str());
    return false;
  }
  if (*size_out < rsp.args(0).size()) {
printf("application_Unseal, output too big\n");
    return false;
  }
  *size_out = rsp.args(0).size();
  memcpy(out, rsp.args(0).data(), *size_out);
  return true;
}

// Attestation is a signed_claim_message
// with a vse_claim_message claim
bool application_Attest(int in_size, byte* in,
  int* size_out, byte* out) {
  app_request req;
  app_response rsp;

  // request
  req.set_function("attest");
  string req_arg_str;
  req_arg_str.assign((char*)in, in_size);
  req.add_args(req_arg_str);
  string req_str;
  req.SerializeToString(&req_str);
  write(writer, (byte*)req_str.data(), req_str.size());

  // response
  int t_size = in_size + 200; // Fix
  byte t_out[t_size];
  int n = read(reader, t_out, t_size);
  if (n < 0)
    return false;
  if (rsp.function() != "attest" || rsp.status() != "succeeded") {
printf("application_Attest, function: %s, status: %s\n", rsp.function().c_str(), rsp.status().c_str());
    return false;
  }
  if (*size_out < rsp.args(0).size()) {
printf("application_Attest, output too big\n");
    return false;
  }
  *size_out = rsp.args(0).size();
  memcpy(out, rsp.args(0).data(), *size_out);
  return true;
}

bool application_Getmeasurement(int* size_out, byte* out) {
  return false;
}