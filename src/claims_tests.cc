#include "certifier.h"
#include "support.h"

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


bool test_claims_1(bool print_all) {
  key_message k;
  if(!make_certifier_rsa_key(1024, &k))
    return false;
  key_message k1;
  if (!private_key_to_public_key(k, &k1))
    return false;
  entity_message e1;
  entity_message e2;
  if (!make_key_entity(k1, &e1))
    return false;
  extern string my_measurement;
  if (!make_measurement_entity(my_measurement, &e2))
    return false;
  vse_clause clause1;
  string s1("is-trusted");
  string s2("says");
  string s3("speaks-for");
  if (!make_unary_vse_clause((const entity_message)e1, s1, &clause1))
    return false;
  vse_clause clause2;
  if (!make_indirect_vse_clause((const entity_message)e1, s2, clause1, &clause2))
    return false;
  vse_clause clause3;
  if (!make_simple_vse_clause((const entity_message)e1, s3, (const entity_message)e2, &clause3))
    return false;

  if (print_all) {
    print_vse_clause(clause1); printf("\n");
    print_vse_clause(clause2); printf("\n");
    print_vse_clause(clause3); printf("\n");
  }

  claim_message full_claim;
  string serialized_claim;
  clause3.SerializeToString(&serialized_claim);
  string f1("vse-clause");
  string d1("basic speaks-for-claim");
  string nb("2021-08-01T05:09:50.000000Z");
  string na("2026-08-01T05:09:50.000000Z");
  if (!make_claim(serialized_claim.size(), (byte*)serialized_claim.data(), f1, d1,
                  nb, na, &full_claim))
    return false;

  if (print_all) {
    printf("\nFull claim:\n");
    print_claim(full_claim);
  }

  claims_sequence seq;
  seq.add_claims();
  if (print_all) {
    printf("Num claims: %d\n", seq.claims_size());
  }
  claim_message* cm = seq.mutable_claims(0);
  cm->CopyFrom(full_claim);
  const claim_message& dm = seq.claims(0);
  if (print_all) {
    printf("\nsequence:\n");
    print_claim(dm);
  }
  return true;
}

bool test_signed_claims(bool print_all) {
  key_message public_attestation_key;
  extern key_message my_attestation_key;
  if (!private_key_to_public_key(my_attestation_key, &public_attestation_key))
    return false;
  entity_message e1;
  entity_message e2;
  if (!make_key_entity(public_attestation_key, &e1))
    return false;

  extern string my_measurement;
  if (!make_measurement_entity(my_measurement, &e2))
    return false;
  string s1("says");
  string s2("speaks-for");
  string vse_clause_format("vse-clause");
  vse_clause clause1;
  vse_clause clause2;
  if (!make_simple_vse_clause((const entity_message)e1, s2, (const entity_message)e2, &clause1))
    return false;
  if (!make_indirect_vse_clause((const entity_message)e1, s2, clause1, &clause2))
    return false;

  string serialized_vse;
  clause2.SerializeToString(&serialized_vse);

  claim_message claim;
  time_point t_nb;
  time_point t_na;
  time_now(&t_nb);
  add_interval_to_time_point(t_nb, 24.0 * 365.0, &t_na);
  string nb;
  string na;
  time_to_string(t_nb, &nb);
  time_to_string(t_na, &na);
  string n1("description");
  if (!make_claim(serialized_vse.size(), (byte*)serialized_vse.data(), vse_clause_format, n1,
    nb, na, &claim))
      return false;
  signed_claim_message signed_claim;
  if(!make_signed_claim(claim, my_attestation_key, &signed_claim))
      return false;
  return verify_signed_claim(signed_claim, public_attestation_key);
}

//  Proofs and certification -----------------------------

// test_support.cc has test code that can be used in an enclave
//    without gtest
#include "test_support.cc"

bool test_certify_steps(bool print_all) {
  return true;
}

bool test_full_certification(bool print_all) {
  return true;
}

// policy-key says intel-key is-trusted-for-attestation
// intel-key says attestation-key is-trusted-for-attestation
// attestation-key says authentication-key speaks-for measurement
// policy-key says measurement is-trusted-for-authentication
// authentication-key is-trusted-for-authentication

const int num_is_trusted_kids = 2;
const char* kids[2] = {
  "is-trusted-for-attestation",
  "is-trusted-for-authentication",
};

bool init_top_level_is_trusted(predicate_dominance& root) {
  root.predicate_.assign("is-trusted");

  string descendant;
  for (int i = 0; i < num_is_trusted_kids; i++) {
    descendant.assign(kids[i]);
    if (!root.insert(root.predicate_, descendant))
      return false;
  }
  return true;
}

bool test_predicate_dominance(bool print_all) {
  predicate_dominance root;

  if (!init_top_level_is_trusted(root)) {
    return false;
  }

  if (print_all) {
    root.print_tree(0);
  }

  string it("is-trusted");
  string it1("is-trusted-for-attestation");
  string it2("is-trusted-for-authentication");
  string it3("is-trusted-for-crap");

  if (!dominates(root, it, it1))
    return false;
  if (!dominates(root, it, it2))
    return false;
  if (dominates(root, it, it3))
    return false;

  return true;
}
