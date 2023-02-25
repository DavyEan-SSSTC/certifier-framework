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
//
// File: simpleserver.go

package main

import (
        "bytes"
        "crypto/x509"
        //"crypto/rsa"
        "flag"
        "fmt"
        "encoding/hex"
        "io/ioutil"
        "log"
        "net"
        "os"
        "strconv"
        "time"

        "github.com/golang/protobuf/proto"
        certprotos "github.com/jlmucb/crypto/v2/certifier-framework-for-confidential-computing/certifier_service/certprotos"
        certlib "github.com/jlmucb/crypto/v2/certifier-framework-for-confidential-computing/certifier_service/certlib"
        //oeverify "github.com/jlmucb/crypto/v2/certifier-framework-for-confidential-computing/certifier_service/oeverify"
)

var serverHost = flag.String("host", "localhost", "address for client/server")
var serverPort = flag.String("port", "8123", "port for client/server")

var policyKeyFile = flag.String("policy_key_file", "policy_key_file.bin", "key file name")
var policyCertFile = flag.String("policy_cert_file", "policy_cert_file.bin", "cert file name")
var readPolicy = flag.Bool("readPolicy", true, "read policy")
var policyFile = flag.String("policyFile", "./certlib/policy.bin", "policy file name")
var loggingSequenceNumber = *flag.Int("loggingSequenceNumber", 1,  "sequence number for logging")

var enableLog = flag.Bool("enableLog", false, "enable logging")
var logDir = flag.String("logDir", ".", "log directory")
var logFile = flag.String("logFile", "simpleserver.log", "log file name")

var privatePolicyKey certprotos.KeyMessage
var publicPolicyKey *certprotos.KeyMessage = nil
var serializedPolicyCert []byte
var policyCert *x509.Certificate = nil
var sn uint64 = uint64(time.Now().UnixNano())
var duration float64 = 365.0 * 86400

var logging bool = false
var logger *log.Logger
var dataPacketFileNum int = loggingSequenceNumber
func initLog() bool {
        name := *logDir + "/" + *logFile
        logFiled, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
        if err != nil {
                fmt.Printf("Can't open log file\n")
                return false
        }
        logger = log.New(logFiled, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
        logger.Println("Starting simpleserver")
        return true
}


var policyInitialized bool = false
var signedPolicyStatements *certprotos.SignedClaimSequence = nil

// There are really only two kinds of initialzed policy now:
//      policy-key says Measurement[] is-trusted
//      policy-key says Key[] is-trusted-for-attestation

type measurementPolicyStatement struct {
        m []byte
        sc certprotos.SignedClaimMessage
}
type platformPolicyStatement struct {
        pk  certprotos.KeyMessage
        sc certprotos.SignedClaimMessage
}

// These are the policy approved program measurements and platform keys.
var measurementList []measurementPolicyStatement
var platformList []platformPolicyStatement

func findPolicyFromMeasurement(m []byte) *certprotos.SignedClaimMessage {
        for i := 0; i < len(measurementList); i++ {
                if bytes.Equal(m, measurementList[i].m) {
                        return &measurementList[i].sc
                }
        }
        return nil
}

func findPolicyFromKey(k *certprotos.KeyMessage) *certprotos.SignedClaimMessage {
        for i := 0; i < len(platformList); i++ {
                if certlib.SameKey(k, &platformList[i].pk) {
                        return &platformList[i].sc
                }
        }
        return nil
}

func initPolicy(thePolicyFile string) bool {

        // Debug
        fmt.Printf("initPolicy\n")

        policySeq, err := os.ReadFile(thePolicyFile)
        if err != nil {
                fmt.Println("can't read policy file, ", err)
                return false
        }

        // Debug
        fmt.Printf("Read %d bytes\n", len(policySeq))

        var  claimBlocks *certprotos.BufferSequence = &certprotos.BufferSequence{}
        err = proto.Unmarshal(policySeq, claimBlocks)
        if err != nil {
                fmt.Println("can't parse policy file, ", err)
                return false
        }

        // Debug
        fmt.Printf("%d policy statements\n", len (claimBlocks.Block))

        for i := 0; i < len(claimBlocks.Block); i++ {
                var sc *certprotos.SignedClaimMessage =  &certprotos.SignedClaimMessage{}
                err = proto.Unmarshal(claimBlocks.Block[i], sc)
                if err != nil {
                        fmt.Println("can't recover policy rule, ", err)
                        return false
                }
                vse := certlib.GetVseFromSignedClaim(sc)
                if vse == nil {
                        continue
                }
                if vse.Subject == nil || vse.Verb == nil || vse.Clause == nil {
                        continue
                }
                if *vse.Verb != "says" {
                        continue
                }
                if vse.Clause.Subject ==nil || vse.Clause.Verb == nil {
                        continue
                }

                if *vse.Clause.Verb == "is-trusted-for-attestation" &&
                                vse.Clause.Subject.GetEntityType() == "key" {
                        ps := platformPolicyStatement {
                                pk: *vse.Clause.Subject.Key,
                                sc:  *sc,
                        }
                        platformList = append(platformList, ps)
                } else if  *vse.Clause.Verb == "is-trusted" &&
                        vse.Clause.Subject.GetEntityType() == "measurement" {
                        ps := measurementPolicyStatement {
                                m: vse.Clause.Subject.Measurement,
                                sc:  *sc,
                        }
                        measurementList = append(measurementList, ps)
                } else {
                        continue
                }
        }

        // Debug
        fmt.Printf("\nMeasurement list, %d entries:\n", len(measurementList))
        for i := 0; i < len(measurementList); i++ {
                fmt.Printf("\n")
                certlib.PrintBytes(measurementList[i].m)
                fmt.Printf("\n")
                certlib.PrintSignedClaim(&measurementList[i].sc)
                fmt.Printf("\n")
        }
        fmt.Printf("\nPlatform list, %d entries:\n", len(platformList))
        for i := 0; i < len(platformList); i++ {
                fmt.Printf("\n")
                certlib.PrintKey(&platformList[i].pk)
                fmt.Printf("\n")
                certlib.PrintSignedClaim(&platformList[i].sc)
                fmt.Printf("\n")
        }
        return true
}

// At init, we retrieve the policy key and the rules to evaluate
func initCertifierService() bool {
        // Debug
        fmt.Printf("initCertifierService, Policy key file: %s, Policy cert file: %s\n", *policyKeyFile, *policyCertFile)

        if *enableLog {
                logging = initLog()
        }

        serializedKey, err := os.ReadFile(*policyKeyFile)
        if err != nil {
                fmt.Println("can't read key file, ", err)
                return false
        }

        serializedPolicyCert, err := os.ReadFile(*policyCertFile)
        if err != nil {
                fmt.Println("can't certkey file, ", err)
        }
        policyCert, err = x509.ParseCertificate(serializedPolicyCert)
        if err != nil {
                fmt.Println("Can't Parse policy cert, ", err)
                return false
        }

        err = proto.Unmarshal(serializedKey, &privatePolicyKey)
        if err != nil {
                return false
        }

        publicPolicyKey = certlib.InternalPublicFromPrivateKey(&privatePolicyKey)
        if publicPolicyKey == nil {
                return false
        }

        if *readPolicy && policyFile != nil {
                policyInitialized = initPolicy(*policyFile)
                if !policyInitialized {
                        fmt.Printf("Error: Couldn't initialize policy\n")
                        return false
                }
        } else {
                fmt.Printf("Error: readPolicy must be true\n")
                return false
        }

        if !certlib.InitSimulatedEnclave() {
                return false
        }
        return true
}

func AddFactFromSignedClaim(signedClaim *certprotos.SignedClaimMessage,
                alreadyProved *certprotos.ProvedStatements) bool {

        k := signedClaim.SigningKey
        tcl := certprotos.VseClause{}
        if certlib.VerifySignedAssertion(*signedClaim, k, &tcl) {
                // make sure the saying key in tcl is the same key that signed it
                if tcl.GetVerb() == "says" && tcl.GetSubject().GetEntityType() == "key" {
                        if certlib.SameKey(k, tcl.GetSubject().GetKey()) {
                                alreadyProved.Proved = append(alreadyProved.Proved, &tcl)
                        } else {
                                return false
                        }
                }
        } else {
                return false
        }
        return true
}

func AddNewFactsForOePlatformAttestation(publicPolicyKey *certprotos.KeyMessage, alreadyProved *certprotos.ProvedStatements) bool {
        // At this point, the already_proved should be
        //    "policyKey is-trusted"
	//    "The platform-key says the enclave-key speaks-for the measurement"
	// Add
	//    "The policy-key says the measurement is-trusted"
	//    "The policy-key says the platform-key is-trusted-for-attestation"
	if len(alreadyProved.Proved) < 2 {
		fmt.Printf("AddNewFactsForOeEvidence, too few initial facts\n")
		return false
	}
	mc := alreadyProved.Proved[1]
	if mc.Subject == nil || mc.Verb == nil || mc.Clause == nil {
		fmt.Printf("AddNewFactsForOeEvidence, bad measurement evidence(1)\n")
		return false
	}
	if mc.Clause.Object == nil {
		fmt.Printf("AddNewFactsForOeEvidence, bad measurement evidence (2)\n")
		return false
	}
	if mc.Clause.Object.GetEntityType() != "measurement" {
		fmt.Printf("AddNewFactsForOeEvidence, bad measurement evidence (3)\n")
		return false
	}
	prog_m := mc.Clause.Object.Measurement
	if prog_m == nil {
		fmt.Printf("AddNewFactsForOeEvidence, bad measurement\n")
		return false
	}

	// Get platformKey from  "The platform-key says the enclave-key speaks-for the measurement"
	kc := alreadyProved.Proved[1]
	if kc.Subject == nil || kc.Verb == nil || kc.Clause == nil {
		fmt.Printf("AddNewFactsForOeEvidence, bad platform evidence(1)\n")
		return false
	}
	if kc.Subject.GetEntityType() != "key" {
		fmt.Printf("AddNewFactsForOeEvidence, bad platform evidence(2)\n")
		return false
	}
	plat_key := kc.Subject.Key
	if plat_key == nil {
		fmt.Printf("AddNewFactsForOeEvidence, bad platform key\n")
		return false
	}

	signedPolicyKeySaysMeasurementIsTrusted := findPolicyFromMeasurement(prog_m)
	if signedPolicyKeySaysMeasurementIsTrusted == nil {
		fmt.Printf("AddNewFactsForOeEvidence, can't find measurement policy\n")
		fmt.Printf("    Measurement: ")
		certlib.PrintBytes(prog_m)
		fmt.Printf("\n")
		return false
	}

	signedPolicyKeySaysPlatformKeyIsTrusted := findPolicyFromKey(plat_key)
	if signedPolicyKeySaysPlatformKeyIsTrusted == nil {
		fmt.Printf("AddNewFactsForOeEvidence, can't find platform policy\n")
		return false
	}

	if !AddFactFromSignedClaim(signedPolicyKeySaysMeasurementIsTrusted, alreadyProved) {
		fmt.Printf("AddNewFactsForOeEvidence, Couldn't AddFactFromSignedClaim, Error 1\n")
		return false
	}

	if !AddFactFromSignedClaim(signedPolicyKeySaysPlatformKeyIsTrusted, alreadyProved) {
		fmt.Printf("AddNewFactsForOeEvidence, Couldn't AddFactFromSignedClaim, Error 2\n")
		return false
	}

	return true
}

func AddNewFactsForGraminePlatformAttestation(publicPolicyKey *certprotos.KeyMessage, alreadyProved *certprotos.ProvedStatements) bool {
        // At this point, the already_proved should be
        //    "policyKey is-trusted"
	//    "The platform-key says the enclave-key speaks-for the measurement"
	// Add
	//    "The policy-key says the measurement is-trusted"
	//    "The policy-key says the platform-key is-trusted-for-attestation"
	if len(alreadyProved.Proved) < 2 {
		fmt.Printf("AddNewFactsForGramineEvidence, too few initial facts\n")
		return false
	}
	mc := alreadyProved.Proved[1]
	if mc.Subject == nil || mc.Verb == nil || mc.Clause == nil {
		fmt.Printf("AddNewFactsForGramineEvidence, bad measurement evidence(1)\n")
		return false
	}
	if mc.Clause.Object == nil {
		fmt.Printf("AddNewFactsForGramineEvidence, bad measurement evidence (2)\n")
		return false
	}
	if mc.Clause.Object.GetEntityType() != "measurement" {
		fmt.Printf("AddNewFactsForGramineEvidence, bad measurement evidence (3)\n")
		return false
	}
	prog_m := mc.Clause.Object.Measurement
	if prog_m == nil {
		fmt.Printf("AddNewFactsForGramineEvidence, bad measurement\n")
		return false
	}
/*
	// Get platformKey from  "The platform-key says the enclave-key speaks-for the measurement"
	kc := alreadyProved.Proved[1]
	if kc.Subject == nil || kc.Verb == nil || kc.Clause == nil {
		fmt.Printf("AddNewFactsForGramineEvidence, bad platform evidence(1)\n")
		return false
	}
	if kc.Subject.GetEntityType() != "key" {
		fmt.Printf("AddNewFactsForGramineEvidence, bad platform evidence(2)\n")
		return false
	}
	plat_key := kc.Subject.Key
	if plat_key == nil {
		fmt.Printf("AddNewFactsForGramineEvidence, bad platform key\n")
		return false
	}
*/
	//TODO: REMOVE
	signedPolicyKeySaysMeasurementIsTrusted := findPolicyFromMeasurement(measurementList[0].m)
	//signedPolicyKeySaysMeasurementIsTrusted := findPolicyFromMeasurement(prog_m)
	if signedPolicyKeySaysMeasurementIsTrusted == nil {
		fmt.Printf("AddNewFactsForGramineEvidence, can't find measurement policy\n")
		fmt.Printf("    Measurement: ")
		certlib.PrintBytes(prog_m)
		fmt.Printf("\n")
		return false
	}
/*
	signedPolicyKeySaysPlatformKeyIsTrusted := findPolicyFromKey(plat_key)
	if signedPolicyKeySaysPlatformKeyIsTrusted == nil {
		fmt.Printf("AddNewFactsForGramineEvidence, can't find platform policy\n")
		return false
	}
*/
	if !AddFactFromSignedClaim(signedPolicyKeySaysMeasurementIsTrusted, alreadyProved) {
		fmt.Printf("AddNewFactsForGramineEvidence, Couldn't AddFactFromSignedClaim, Error 1\n")
		return false
	}
/*
	if !AddFactFromSignedClaim(signedPolicyKeySaysPlatformKeyIsTrusted, alreadyProved) {
		fmt.Printf("AddNewFactsForGramineEvidence, Couldn't AddFactFromSignedClaim, Error 2\n")
		return false
	}
*/
	return true
}

func AddNewFactsForSevEvidence(publicPolicyKey *certprotos.KeyMessage,
                alreadyProved *certprotos.ProvedStatements) bool {
        // At this point, the already_proved should be
        //    "policyKey is-trusted"
        //    "The ARK-key says the ARK-key is-trusted-for-attestation"
        //    "The ARK-key says the ASK-key is-trusted-for-attestation"
        //    "The ASK-key says the VCEK-key is-trusted-for-attestation"
        //    "VCEK says the enclave-key speaks-for the measurement"
        // Add
        //    "The policy-key says the ARK-key is-trusted-for-attestation"
        //    "The policy-key says the measurement is-trusted"

        // Get measurement from  "VCEK says the enclave-key speaks-for the measurement"
	if len(alreadyProved.Proved) < 5 {
                fmt.Printf("AddNewFactsForSevEvidence, too few initial facts\n")
                return false
	}
        mc := alreadyProved.Proved[4]
        if mc.Subject == nil || mc.Verb == nil || mc.Clause == nil {
                fmt.Printf("AddNewFactsForSevEvidence, bad measurement evidence(1)\n")
                return false
        }
        if mc.Clause.Object == nil {
                fmt.Printf("AddNewFactsForSevEvidence, bad measurement evidence (2)\n")
                return false
        }
        if mc.Clause.Object.GetEntityType() != "measurement" {
                fmt.Printf("AddNewFactsForSevEvidence, bad measurement evidence (3)\n")
                return false
        }
        prog_m := mc.Clause.Object.Measurement
        if prog_m == nil {
                fmt.Printf("AddNewFactsForSevEvidence, bad measurement\n")
                return false
        }

        // Get platformKey from  "The ARK-key says the ARK-key is-trusted-for-attestation"
        kc := alreadyProved.Proved[1]
        if kc.Subject == nil || kc.Verb == nil || kc.Clause == nil {
                fmt.Printf("AddNewFactsForSevEvidence, bad platform evidence(1)\n")
                return false
        }
        if kc.Subject.GetEntityType() != "key" {
                fmt.Printf("AddNewFactsForSevEvidence, bad platform evidence(2)\n")
                return false
        }
        plat_key := kc.Subject.Key
        if plat_key == nil {
                fmt.Printf("AddNewFactsForSevEvidence, bad platform key\n")
                return false
        }

        signedPolicyKeySaysMeasurementIsTrusted := findPolicyFromMeasurement(prog_m)
        if signedPolicyKeySaysMeasurementIsTrusted == nil {
                fmt.Printf("AddNewFactsForSevEvidence, can't find measurement policy\n")
                return false
        }

        signedPolicyKeySaysPlatformKeyIsTrusted := findPolicyFromKey(plat_key)
        if signedPolicyKeySaysPlatformKeyIsTrusted == nil {
                fmt.Printf("AddNewFactsForSevEvidence, can't find platform policy\n")
                return false
        }

        if !AddFactFromSignedClaim(signedPolicyKeySaysPlatformKeyIsTrusted, alreadyProved) {
                fmt.Printf("Couldn't AddFactFromSignedClaim, Error 2\n")
                return false
        }

        if !AddFactFromSignedClaim(signedPolicyKeySaysMeasurementIsTrusted, alreadyProved) {
                fmt.Printf("Couldn't AddFactFromSignedClaim, Error 1\n")
                return false
        }

        return true
}

func AddNewFactsForAbbreviatedPlatformAttestation(publicPolicyKey *certprotos.KeyMessage,
        alreadyProved *certprotos.ProvedStatements) bool {

        // At this point, already proved should contain
        //      "policyKey is-trusted"
        //      "platformKey says attestationKey is-trusted-for-attestation
        //      "attestKey says enclaveKey speaks-for measurement"
        // Add
        //      "policyKey says platformKey is-trusted-for-attestation"
        //      "policyKey says measurement is-trusted"
        // Get platform measurement statement and platform statement
        // Find the corresponding "measurement is-trusted" in measurements
        //      This is signedPolicyKeySaysMeasurementIsTrusted
        //      Add it
        // Find the corresponding "platformKey is-trusted" in platforms 
        //      This is signedPolicyKeySaysPlatformKeyIsTrusted
        //      Add it
        if len(alreadyProved.Proved) != 3 {
                fmt.Printf("AddNewFactsForAbbreviatedPlatformAttestation, Error 1\n")
                return false
        }

        // Get measurement from "attestKey says enclaveKey speaks-for measurement"
        mc := alreadyProved.Proved[2]
        if mc.Subject == nil || mc.Verb == nil || mc.Clause == nil {
                fmt.Printf("AddNewFactsForAbbreviatedPlatformAttestation, bad measurement evidence(1)\n")
                return false
        }
        if mc.Clause.Object == nil {
                fmt.Printf("AddNewFactsForAbbreviatedPlatformAttestation, bad measurement evidence (2)\n")
                return false
        }
        if mc.Clause.Object.GetEntityType() != "measurement" {
                fmt.Printf("AddNewFactsForAbbreviatedPlatformAttestation, bad measurement evidence (3)\n")
                return false
        }
        prog_m := mc.Clause.Object.Measurement

        // Get platformKey from "platformKey says attestationKey is-trusted-for-attestation
        kc := alreadyProved.Proved[1]
        if kc.Subject == nil || kc.Verb == nil || kc.Clause == nil {
                fmt.Printf("AddNewFactsForAbbreviatedPlatformAttestation, bad platform evidence(1)\n")
                return false
        }
        if kc.Subject.GetEntityType() != "key" {
                fmt.Printf("AddNewFactsForAbbreviatedPlatformAttestation, bad platform evidence(2)\n")
                return false
        }
        plat_key := kc.Subject.Key

        signedPolicyKeySaysMeasurementIsTrusted := findPolicyFromMeasurement(prog_m)
        if signedPolicyKeySaysMeasurementIsTrusted == nil {
                fmt.Printf("AddNewFactsForAbbreviatedPlatformAttestation, can't find measurement policy\n")
                return false
        }
        signedPolicyKeySaysPlatformKeyIsTrusted := findPolicyFromKey(plat_key)
        if signedPolicyKeySaysMeasurementIsTrusted == nil {
                fmt.Printf("AddNewFactsForAbbreviatedPlatformAttestation, can't find platform policy\n")
                return false
        }

        // Debug
        fmt.Printf("\nPolicy:\n")
        certlib.PrintSignedClaim(signedPolicyKeySaysMeasurementIsTrusted)
        fmt.Printf("\n")
        certlib.PrintSignedClaim(signedPolicyKeySaysPlatformKeyIsTrusted)
        fmt.Printf("\n")

        if !AddFactFromSignedClaim(signedPolicyKeySaysPlatformKeyIsTrusted, alreadyProved) {
                fmt.Printf("Couldn't AddFactFromSignedClaim, Error 2\n")
                return false
        }

        if !AddFactFromSignedClaim(signedPolicyKeySaysMeasurementIsTrusted, alreadyProved) {
                fmt.Printf("Couldn't AddFactFromSignedClaim, Error 1\n")
                return false
        }

        return true
}

func AddNewFactsForAugmentedPlatformAttestation(publicPolicyKey *certprotos.KeyMessage,
        alreadyProved *certprotos.ProvedStatements) bool {

        // At this point, already proved should contain
        //      "policyKey is-trusted"
        //      "policyKey says attestKey is-trusted-for-attestation"
        //      "attestKey says enclaveKey speaks-for measurement"
        // Add
        //      "policyKey says measurement is-trusted"
        // Get platform measurement statement and platform statement
        // Find the corresponding "measurement is-trusted" in measurements
        //      This is signedPolicyKeySaysMeasurementIsTrusted
        //      Add it
        if len(alreadyProved.Proved) != 3 {
                fmt.Printf("AddNewFactsForAugmentedPlatformAttestation, Error 1\n")
                return false
        }

        // Get measurement from "attestKey says enclaveKey speaks-for measurement"
        mc := alreadyProved.Proved[2]
        if mc.Subject == nil || mc.Verb == nil || mc.Clause == nil {
                fmt.Printf("AddNewFactsForAugmentedPlatformAttestation, bad measurement evidence(1)\n")
                return false
        }
        if mc.Clause.Object == nil {
                fmt.Printf("AddNewFactsForAugmentedPlatformAttestation, bad measurement evidence (2)\n")
                return false
        }
        if mc.Clause.Object.GetEntityType() != "measurement" {
                fmt.Printf("AddNewFactsForAugmentedPlatformAttestation, bad measurement evidence (3)\n")
                return false
        }
        prog_m := mc.Clause.Object.Measurement

        signedPolicyKeySaysMeasurementIsTrusted := findPolicyFromMeasurement(prog_m)
        if signedPolicyKeySaysMeasurementIsTrusted == nil {
                fmt.Printf("AddNewFactsForAugmentedPlatformAttestation, can't find measurement policy\n")
                return false
        }

        // Debug
        fmt.Printf("\nPolicy:\n")
        certlib.PrintSignedClaim(signedPolicyKeySaysMeasurementIsTrusted)
        fmt.Printf("\n")

        if !AddFactFromSignedClaim(signedPolicyKeySaysMeasurementIsTrusted, alreadyProved) {
                fmt.Printf("Couldn't AddFactFromSignedClaim, Error 1\n")
                return false
        }

        return true
}

// Returns toProve and proof steps
func ConstructProofFromOeEvidence(publicPolicyKey *certprotos.KeyMessage, purpose string, alreadyProved certprotos.ProvedStatements) (*certprotos.VseClause, *certprotos.Proof) {

        // At this point, the evidence should be
        //      "policyKey is-trusted"
        //      "platform-key says enclaveKey speaks-for measurement
        //      "policyKey says measurement is-trusted"
        //      "policyKey says platformKey is-trusted-for-attestation"

	// Debug
	fmt.Printf("ConstructProofFromOeEvidence, %d statements\n", len(alreadyProved.Proved))
	for i := 0; i < len(alreadyProved.Proved);  i++ {
		certlib.PrintVseClause(alreadyProved.Proved[i])
		fmt.Printf("\n")
	}

	if len(alreadyProved.Proved) < 4 {
		fmt.Printf("ConstructProofFromOeEvidence: too few statements\n")
		return nil, nil
	}
	policyKeyIsTrusted :=  alreadyProved.Proved[0]
	platformSaysEnclaveKeySpeaksForMeasurement :=  alreadyProved.Proved[1]
	if platformSaysEnclaveKeySpeaksForMeasurement.Clause == nil {
		fmt.Printf("ConstructProofFromOeEvidence: can't get enclaveKeySpeaksForMeasurement\n")
		return nil, nil
	}
	enclaveKeySpeaksForMeasurement :=  platformSaysEnclaveKeySpeaksForMeasurement.Clause
	policyKeySaysMeasurementIsTrusted :=  alreadyProved.Proved[2]
	if policyKeyIsTrusted == nil || enclaveKeySpeaksForMeasurement == nil ||
			policyKeySaysMeasurementIsTrusted == nil {
		fmt.Printf("ConstructProofFromOeEvidence: Error 4\n")
		return nil, nil
	}

	policyKeySaysPlatformKeyIsTrustedForAttestation := alreadyProved.Proved[3]
	if policyKeySaysPlatformKeyIsTrustedForAttestation.Clause == nil {
		fmt.Printf("ConstructProofFromOeEvidence: Can't get platformKeyIsTrustedForAttestation\n")
		return nil, nil
	}
	platformKeyIsTrustedForAttestation := policyKeySaysPlatformKeyIsTrustedForAttestation.Clause

        proof := &certprotos.Proof{}
        r1 := int32(1)
        r3 := int32(3)
        r6 := int32(6)
        r7 := int32(7)

	enclaveKey := enclaveKeySpeaksForMeasurement.Subject
	if enclaveKey == nil || enclaveKey.GetEntityType() != "key" {
		fmt.Printf("ConstructProofFromOeEvidence: Bad enclave key\n")
		return nil, nil
	}
        var toProve *certprotos.VseClause = nil
	if purpose == "authentication" {
		verb := "is-trusted-for-authentication"
		toProve = certlib.MakeUnaryVseClause(enclaveKey, &verb)
	} else {
		verb := "is-trusted-for-attestation"
		toProve = certlib.MakeUnaryVseClause(enclaveKey, &verb)
	}

	measurementIsTrusted := policyKeySaysMeasurementIsTrusted.Clause
	if measurementIsTrusted == nil {
		fmt.Printf("ConstructProofFromOeEvidence: Can't get measurement\n")
		return nil, nil
	}
	ps1 := certprotos.ProofStep {
		S1: policyKeyIsTrusted,
		S2: policyKeySaysMeasurementIsTrusted,
		Conclusion: measurementIsTrusted,
		RuleApplied: &r3,
	}
	proof.Steps = append(proof.Steps, &ps1)

	ps2 := certprotos.ProofStep {
		S1: policyKeyIsTrusted,
		S2: policyKeySaysPlatformKeyIsTrustedForAttestation,
		Conclusion: platformKeyIsTrustedForAttestation,
		RuleApplied: &r3,
	}
	proof.Steps = append(proof.Steps, &ps2)

	ps3 := certprotos.ProofStep {
		S1: platformKeyIsTrustedForAttestation,
		S2: platformSaysEnclaveKeySpeaksForMeasurement,
		Conclusion: enclaveKeySpeaksForMeasurement,
		RuleApplied: &r6,
	}
	proof.Steps = append(proof.Steps, &ps3)

	// measurement is-trusted and enclaveKey speaks-for measurement -->
	//	enclaveKey is-trusted-for-authentication (r1) or
	//	enclaveKey is-trusted-for-attestation (r7)
	if purpose == "authentication" {
		ps4 := certprotos.ProofStep {
			S1: measurementIsTrusted,
			S2: enclaveKeySpeaksForMeasurement,
			Conclusion: toProve,
			RuleApplied: &r1,
		}
		proof.Steps = append(proof.Steps, &ps4)
	} else {
		ps4 := certprotos.ProofStep {
			S1: measurementIsTrusted,
			S2: enclaveKeySpeaksForMeasurement,
			Conclusion: toProve,
			RuleApplied: &r7,
		}
		proof.Steps = append(proof.Steps, &ps4)
	}

        return toProve, proof
}

// Returns toProve and proof steps
func ConstructProofFromGramineEvidence(publicPolicyKey *certprotos.KeyMessage, purpose string, alreadyProved certprotos.ProvedStatements) (*certprotos.VseClause, *certprotos.Proof) {

        // At this point, the evidence should be
        //      "policyKey is-trusted"
        //      "platform-key says enclaveKey speaks-for measurement
        //      "policyKey says measurement is-trusted"
        //      "policyKey says platformKey is-trusted-for-attestation"

	// Debug
	fmt.Printf("ConstructProofFromGramineEvidence, %d statements\n", len(alreadyProved.Proved))
	for i := 0; i < len(alreadyProved.Proved);  i++ {
		certlib.PrintVseClause(alreadyProved.Proved[i])
		fmt.Printf("\n")
	}

	//if len(alreadyProved.Proved) < 4 {
	if len(alreadyProved.Proved) < 3 {
		fmt.Printf("ConstructProofFromGramineEvidence: too few statements\n")
		return nil, nil
	}
	policyKeyIsTrusted :=  alreadyProved.Proved[0]
	platformSaysEnclaveKeySpeaksForMeasurement :=  alreadyProved.Proved[1]
	if platformSaysEnclaveKeySpeaksForMeasurement.Clause == nil {
		fmt.Printf("ConstructProofFromGramineEvidence: can't get enclaveKeySpeaksForMeasurement\n")
		return nil, nil
	}
	enclaveKeySpeaksForMeasurement :=  platformSaysEnclaveKeySpeaksForMeasurement.Clause
	policyKeySaysMeasurementIsTrusted :=  alreadyProved.Proved[2]
	if policyKeyIsTrusted == nil || enclaveKeySpeaksForMeasurement == nil ||
			policyKeySaysMeasurementIsTrusted == nil {
		fmt.Printf("ConstructProofFromGramineEvidence: Error 4\n")
		return nil, nil
	}
/*
	policyKeySaysPlatformKeyIsTrustedForAttestation := alreadyProved.Proved[3]
	if policyKeySaysPlatformKeyIsTrustedForAttestation.Clause == nil {
		fmt.Printf("ConstructProofFromGramineEvidence: Can't get platformKeyIsTrustedForAttestation\n")
		return nil, nil
	}
	platformKeyIsTrustedForAttestation := policyKeySaysPlatformKeyIsTrustedForAttestation.Clause
*/
        proof := &certprotos.Proof{}
        r1 := int32(1)
        r3 := int32(3)
        //r6 := int32(6)
        r7 := int32(7)

	enclaveKey := enclaveKeySpeaksForMeasurement.Subject
	if enclaveKey == nil || enclaveKey.GetEntityType() != "key" {
		fmt.Printf("ConstructProofFromGramineEvidence: Bad enclave key\n")
		return nil, nil
	}
        var toProve *certprotos.VseClause = nil
	if purpose == "authentication" {
		verb := "is-trusted-for-authentication"
		toProve = certlib.MakeUnaryVseClause(enclaveKey, &verb)
	} else {
		verb := "is-trusted-for-attestation"
		toProve = certlib.MakeUnaryVseClause(enclaveKey, &verb)
	}

	measurementIsTrusted := policyKeySaysMeasurementIsTrusted.Clause
	if measurementIsTrusted == nil {
		fmt.Printf("ConstructProofFromGramineEvidence: Can't get measurement\n")
		return nil, nil
	}
	ps1 := certprotos.ProofStep {
		S1: policyKeyIsTrusted,
		S2: policyKeySaysMeasurementIsTrusted,
		Conclusion: measurementIsTrusted,
		RuleApplied: &r3,
	}
	proof.Steps = append(proof.Steps, &ps1)
/*
	ps2 := certprotos.ProofStep {
		S1: policyKeyIsTrusted,
		S2: policyKeySaysPlatformKeyIsTrustedForAttestation,
		Conclusion: platformKeyIsTrustedForAttestation,
		RuleApplied: &r3,
	}
	proof.Steps = append(proof.Steps, &ps2)

	ps3 := certprotos.ProofStep {
		S1: platformKeyIsTrustedForAttestation,
		S2: platformSaysEnclaveKeySpeaksForMeasurement,
		Conclusion: enclaveKeySpeaksForMeasurement,
		RuleApplied: &r6,
	}
	proof.Steps = append(proof.Steps, &ps3)
*/
	// measurement is-trusted and enclaveKey speaks-for measurement -->
	//	enclaveKey is-trusted-for-authentication (r1) or
	//	enclaveKey is-trusted-for-attestation (r7)
	if purpose == "authentication" {
		ps4 := certprotos.ProofStep {
			S1: measurementIsTrusted,
			S2: enclaveKeySpeaksForMeasurement,
			Conclusion: toProve,
			RuleApplied: &r1,
		}
		proof.Steps = append(proof.Steps, &ps4)
	} else {
		ps4 := certprotos.ProofStep {
			S1: measurementIsTrusted,
			S2: enclaveKeySpeaksForMeasurement,
			Conclusion: toProve,
			RuleApplied: &r7,
		}
		proof.Steps = append(proof.Steps, &ps4)
	}

        return toProve, proof
}

// Returns toProve and proof steps
func ConstructProofFromFullVseEvidence(publicPolicyKey *certprotos.KeyMessage, purpose string,
        alreadyProved certprotos.ProvedStatements) (*certprotos.VseClause, *certprotos.Proof) {

        // At this point, alreadyProved should be
        //      "policyKey is-trusted"
        //      "platformKey says attestationKey is-trusted-for-attestation"
        //      "attestKey says enclaveKey speaks-for measurement
        //      "policyKey says platformKey is-trusted-for-attestation"
        //      "policyKey says measurement is-trusted"

        // Debug
        fmt.Printf("ConstructProofFromFullVseEvidence entries %d\n", len(alreadyProved.Proved))

        proof := &certprotos.Proof{}
        r1 := int32(1)
        r3 := int32(3)
        r5 := int32(5)
        r6 := int32(6)
        r7 := int32(7)

        policyKeyIsTrusted := alreadyProved.Proved[0]
        policyKeySaysMeasurementIsTrusted := alreadyProved.Proved[4]
        measurementIsTrusted := policyKeySaysMeasurementIsTrusted.Clause
        ps1 := certprotos.ProofStep {
                S1: policyKeyIsTrusted,
                S2: policyKeySaysMeasurementIsTrusted,
                Conclusion: measurementIsTrusted,
                RuleApplied: &r3,
        }
        proof.Steps = append(proof.Steps, &ps1)

        policyKeySaysPlatformKeyIsTrusted := alreadyProved.Proved[3]
        platformKeyIsTrusted := policyKeySaysPlatformKeyIsTrusted.Clause
        ps2 := certprotos.ProofStep {
                S1: policyKeyIsTrusted,
                S2: policyKeySaysPlatformKeyIsTrusted,
                Conclusion: platformKeyIsTrusted,
                RuleApplied: &r5,
        }
        proof.Steps = append(proof.Steps, &ps2)

        platformKeySaysAttestKeyIsTrusted := alreadyProved.Proved[1]
        attestKeyIsTrusted := platformKeySaysAttestKeyIsTrusted.Clause
        ps3 := certprotos.ProofStep {
                S1: platformKeyIsTrusted,
                S2: platformKeySaysAttestKeyIsTrusted,
                Conclusion: attestKeyIsTrusted,
                RuleApplied: &r5,
        }
        proof.Steps = append(proof.Steps, &ps3)

        attestKeySaysEnclaveKeySpeaksForMeasurement := alreadyProved.Proved[2]
        enclaveKeySpeaksForMeasurement := attestKeySaysEnclaveKeySpeaksForMeasurement.Clause
        ps4 := certprotos.ProofStep {
        S1: attestKeyIsTrusted,
        S2: attestKeySaysEnclaveKeySpeaksForMeasurement,
        Conclusion: enclaveKeySpeaksForMeasurement,
        RuleApplied: &r6,
        }
        proof.Steps = append(proof.Steps, &ps4)

        var toProve *certprotos.VseClause = nil
        isTrustedForAuth := "is-trusted-for-authentication"
        isTrustedForAttest:= "is-trusted-for-attestation"
        if  purpose == "attestation" {
                toProve =  certlib.MakeUnaryVseClause(enclaveKeySpeaksForMeasurement.Subject,
                        &isTrustedForAttest)
                ps5 := certprotos.ProofStep {
                S1: measurementIsTrusted,
                S2: enclaveKeySpeaksForMeasurement,
                Conclusion: toProve,
                RuleApplied: &r7,
                }
                proof.Steps = append(proof.Steps, &ps5)
        } else {
                toProve =  certlib.MakeUnaryVseClause(enclaveKeySpeaksForMeasurement.Subject,
                        &isTrustedForAuth)
                ps5 := certprotos.ProofStep {
                S1: measurementIsTrusted,
                S2: enclaveKeySpeaksForMeasurement,
                Conclusion: toProve,
                RuleApplied: &r1,
                }
                proof.Steps = append(proof.Steps, &ps5)
        }

        return toProve, proof
}

// Returns toProve and proof steps
func ConstructProofFromShortVseEvidence(publicPolicyKey *certprotos.KeyMessage, purpose string,
        alreadyProved certprotos.ProvedStatements) (*certprotos.VseClause, *certprotos.Proof) {

        // At this point, alreadyProved should be
        //      "policyKey is-trusted"
        //      "platformKey says attestationKey is-trusted-for-attestation"
        //      "attestKey says enclaveKey speaks-for measurement
        //      "policyKey says measurement is-trusted"

        // Debug
        fmt.Printf("ConstructProofFromFullVseEvidence entries %d\n", len(alreadyProved.Proved))

        proof := &certprotos.Proof{}
        r1 := int32(1)
        r3 := int32(3)
        r5 := int32(5)
        r6 := int32(6)
        r7 := int32(7)

        policyKeyIsTrusted := alreadyProved.Proved[0]
        policyKeySaysMeasurementIsTrusted := alreadyProved.Proved[3]
        measurementIsTrusted := policyKeySaysMeasurementIsTrusted.Clause
        ps1 := certprotos.ProofStep {
                S1: policyKeyIsTrusted,
                S2: policyKeySaysMeasurementIsTrusted,
                Conclusion: measurementIsTrusted,
                RuleApplied: &r3,
        }
        proof.Steps = append(proof.Steps, &ps1)

        policyKeySaysAttestKeyIsTrusted := alreadyProved.Proved[1]
        attestKeyIsTrusted := policyKeySaysAttestKeyIsTrusted.Clause
        ps3 := certprotos.ProofStep {
                S1: policyKeyIsTrusted,
                S2: policyKeySaysAttestKeyIsTrusted,
                Conclusion: attestKeyIsTrusted,
                RuleApplied: &r5,
        }
        proof.Steps = append(proof.Steps, &ps3)

        attestKeySaysEnclaveKeySpeaksForMeasurement := alreadyProved.Proved[2]
        enclaveKeySpeaksForMeasurement := attestKeySaysEnclaveKeySpeaksForMeasurement.Clause
        ps4 := certprotos.ProofStep {
        S1: attestKeyIsTrusted,
        S2: attestKeySaysEnclaveKeySpeaksForMeasurement,
        Conclusion: enclaveKeySpeaksForMeasurement,
        RuleApplied: &r6,
        }
        proof.Steps = append(proof.Steps, &ps4)

        var toProve *certprotos.VseClause = nil
        isTrustedForAuth := "is-trusted-for-authentication"
        isTrustedForAttest:= "is-trusted-for-attestation"
        if  purpose == "attestation" {
                toProve =  certlib.MakeUnaryVseClause(enclaveKeySpeaksForMeasurement.Subject,
                        &isTrustedForAttest)
                ps5 := certprotos.ProofStep {
                S1: measurementIsTrusted,
                S2: enclaveKeySpeaksForMeasurement,
                Conclusion: toProve,
                RuleApplied: &r7,
                }
                proof.Steps = append(proof.Steps, &ps5)
        } else {
                toProve =  certlib.MakeUnaryVseClause(enclaveKeySpeaksForMeasurement.Subject,
                        &isTrustedForAuth)
                ps5 := certprotos.ProofStep {
                S1: measurementIsTrusted,
                S2: enclaveKeySpeaksForMeasurement,
                Conclusion: toProve,
                RuleApplied: &r1,
                }
                proof.Steps = append(proof.Steps, &ps5)
        }

        return toProve, proof
}

func ConstructProofFromSevEvidence(publicPolicyKey *certprotos.KeyMessage,
		purpose string, alreadyProved certprotos.ProvedStatements) (*certprotos.VseClause, *certprotos.Proof) {
        // At this point, the already_proved should be
        //    0 : "policyKey is-trusted"
        //    1: "The ARK-key says the ARK-key is-trusted-for-attestation"
        //    2: "The ARK-key says the ASK-key is-trusted-for-attestation"
        //    3: "The ASK-key says the VCEK-key is-trusted-for-attestation"
        //    4: "VCEK says the enclave-key speaks-for the measurement
        //    5: "The policyKey says the ARK-key is-trusted-for-attestation
        //    6: "policyKey says measurement is-trusted"

        // Proof is:
        //    "policyKey is-trusted" AND policyKey says measurement is-trusted" -->
        //        "the measurement is-trusted" (R3)
        //    "policyKey is-trusted" AND
        //        "policy-key says the ARK-key is-trusted-for-attestation" -->
        //        "the ARK-key is-trusted-for-attestation" (R3)
        //    "the ARK-key is-trusted-for-attestation" AND
        //        "The ARK-key says the ASK-key is-trusted-for-attestation" -->
        //        "the ASK-key is-trusted-for-attestation" (R5)
        //    "the ASK-key is-trusted-for-attestation" AND
        //        "the ASK-key says the VCEK-key is-trusted-for-attestation" -->
        //        "the VCEK-key is-trusted-for-attestation" (R5)
        //    "the VCEK-key is-trusted-for-attestation" AND
        //        "the VCEK-key says the enclave-key speaks-for the measurement" -->
        //        "enclave-key speaks-for the measurement"  (R6)
        //    "enclave-key speaks-for the measurement" AND "the measurement is-trusted" -->
        //        "the enclave key is-trusted-for-authentication" (R1) OR
        //        "the enclave key is-trusted-for-attestation" (R7)

        // Debug
        fmt.Printf("ConstructProofFromSevEvidence entries %d\n", len(alreadyProved.Proved))

        proof := &certprotos.Proof{}
        r1 := int32(1)
        r3 := int32(3)
        r5 := int32(5)
        r6 := int32(6)
        r7 := int32(7)

        if len(alreadyProved.Proved) != 7 {
                fmt.Printf("ConstructProofFromSevEvidence: Wrong number of proved statements\n")
                return nil, nil
        }
        policyKeyIsTrusted := alreadyProved.Proved[0]
        if policyKeyIsTrusted == nil {
                fmt.Printf("ConstructProofFromSevEvidence: Can't get policyKey is trusted\n")
                return nil, nil
        }
        policyKeySaysMeasurementIsTrusted := alreadyProved.Proved[6]
        if policyKeySaysMeasurementIsTrusted == nil  || policyKeySaysMeasurementIsTrusted.Clause == nil {
                fmt.Printf("ConstructProofFromSevEvidence: Can't get measurementIsTrusted (1)\n")
                return nil, nil
        }
        measurementIsTrusted := policyKeySaysMeasurementIsTrusted.Clause
        if measurementIsTrusted == nil {
                fmt.Printf("ConstructProofFromSevEvidence: Can't get measurementIsTrusted (2)\n")
                return nil, nil
        }
        vcertSaysEnclaveKeySpeaksForMeasurement := alreadyProved.Proved[4]
        if vcertSaysEnclaveKeySpeaksForMeasurement == nil {
                fmt.Printf("ConstructProofFromSevEvidence: Can't get attestation\n")
                return nil, nil
        }
        policyKeySaysArkKeyIsTrustedForAttestation := alreadyProved.Proved[5]
        if policyKeySaysArkKeyIsTrustedForAttestation == nil  ||
                        policyKeySaysArkKeyIsTrustedForAttestation.Clause == nil {
                fmt.Printf("ConstructProofFromSevEvidence: Can't get policyKeySaysArkKeyIsTrustedForAttestation\n")
                return nil, nil
        }
        arkIsTrustedForAttestation := policyKeySaysArkKeyIsTrustedForAttestation.Clause
        arkKeySaysAskKeyIsTrustedForAttestation := alreadyProved.Proved[2]
        if arkKeySaysAskKeyIsTrustedForAttestation == nil  ||
                        arkKeySaysAskKeyIsTrustedForAttestation.Clause == nil {
                fmt.Printf("ConstructProofFromSevEvidence: Can't get arkKeySaysAskKeyIsTrustedForAttestation\n")
                return nil, nil
        }
        askKeyIsTrustedForAttestation:= arkKeySaysAskKeyIsTrustedForAttestation.Clause
        askKeySaysVcertKeyIsTrustedForAttestation := alreadyProved.Proved[3]
        if askKeySaysVcertKeyIsTrustedForAttestation == nil  || askKeySaysVcertKeyIsTrustedForAttestation.Clause == nil {
                fmt.Printf("ConstructProofFromSevEvidence: Can't get askKeySaysVcertKeyIsTrustedForAttestation\n")
                return nil, nil
        }
        vcertKeyIsTrusted := askKeySaysVcertKeyIsTrustedForAttestation.Clause
        if vcertKeyIsTrusted == nil {
                fmt.Printf("ConstructProofFromSevEvidence: Can't get vcertKeyIsTrustedForAttestation\n")
                return nil, nil
        }
        enclaveKeySpeaksForMeasurement := vcertSaysEnclaveKeySpeaksForMeasurement.Clause
        if vcertSaysEnclaveKeySpeaksForMeasurement.Clause == nil {
                fmt.Printf("ConstructProofFromSevEvidence: Can't get enclaveKeySpeaksForMeasurement\n")
                return nil, nil
        }

        //    "policyKey is-trusted" AND policyKey says measurement is-trusted" -->
        //        "the measurement is-trusted" (R3)
        ps1 := certprotos.ProofStep {
                S1: policyKeyIsTrusted,
                S2: policyKeySaysMeasurementIsTrusted,
                Conclusion: measurementIsTrusted,
                RuleApplied: &r3,
        }
        proof.Steps = append(proof.Steps, &ps1)

        //    "policyKey is-trusted" AND
        //        "policy-key says the ARK-key is-trusted-for-attestation" -->
        //        "the ARK-key is-trusted-for-attestation" (R5)
        ps2 := certprotos.ProofStep {
                S1: policyKeyIsTrusted,
                S2: policyKeySaysArkKeyIsTrustedForAttestation,
                Conclusion: arkIsTrustedForAttestation,
                RuleApplied: &r5,
        }
        proof.Steps = append(proof.Steps, &ps2)

        //    "the ARK-key is-trusted-for-attestation" AND
        //        "The ARK-key says the ASK-key is-trusted-for-attestation" -->
        //        "the ASK-key is-trusted-for-attestation" (R5)
        ps3 := certprotos.ProofStep {
                S1: arkIsTrustedForAttestation,
                S2: arkKeySaysAskKeyIsTrustedForAttestation,
                Conclusion: askKeyIsTrustedForAttestation,
                RuleApplied: &r5,
        }
        proof.Steps = append(proof.Steps, &ps3)

        //    "the ASK-key is-trusted-for-attestation" AND
        //        "the ASK-key says the VCEK-key is-trusted-for-attestation" -->
        //        "the VCEK-key is-trusted-for-attestation" (R5)
        ps4 := certprotos.ProofStep {
                S1: askKeyIsTrustedForAttestation,
                S2: askKeySaysVcertKeyIsTrustedForAttestation,
                Conclusion: vcertKeyIsTrusted,
                RuleApplied: &r5,
        }
        proof.Steps = append(proof.Steps, &ps4)

        //    "the VCEK-key is-trusted-for-attestation" AND
        //        "the VCEK-key says the enclave-key speaks-for the measurement" -->
        //        "enclave-key speaks-for the measurement"  (R6)
        ps5 := certprotos.ProofStep {
                S1: vcertKeyIsTrusted,
                S2: vcertSaysEnclaveKeySpeaksForMeasurement,
                Conclusion: enclaveKeySpeaksForMeasurement,
                RuleApplied: &r6,
        }
        proof.Steps = append(proof.Steps, &ps5)

        //    "measurement-is-trusted AND "enclave-key speaks-for the measurement" -->
        //        "the enclave key is-trusted-for-authentication" (R1) OR
        //        "the enclave key is-trusted-for-attestation" (R7)
        var toProve *certprotos.VseClause = nil
        isTrustedForAuth := "is-trusted-for-authentication"
        isTrustedForAttest:= "is-trusted-for-attestation"
        if  purpose == "attestation" {
                toProve =  certlib.MakeUnaryVseClause(enclaveKeySpeaksForMeasurement.Subject,
                        &isTrustedForAttest)
                ps6 := certprotos.ProofStep {
                        S1: measurementIsTrusted,
                        S2: enclaveKeySpeaksForMeasurement,
                        Conclusion: toProve,
                        RuleApplied: &r7,
                }
                proof.Steps = append(proof.Steps, &ps6)
        } else {
                toProve =  certlib.MakeUnaryVseClause(enclaveKeySpeaksForMeasurement.Subject,
                        &isTrustedForAuth)
                ps7 := certprotos.ProofStep {
                        S1: measurementIsTrusted,
                        S2: enclaveKeySpeaksForMeasurement,
                        Conclusion: toProve,
                        RuleApplied: &r1,
                }
                proof.Steps = append(proof.Steps, &ps7)
        }

        return toProve, proof
}


//      ConstructProofFromRequest first checks evidence and make sure each evidence
//            component is verified and it put in alreadyProved Statements
//      Next, alreadyProved is augmented to include additional true statements
//            required for the proof
//      Finally a proof is constructed
//
//      Returns the proof goal (toProve), the proof steps (proof), 
//            and a list of true statements (alreadyProved)
func ConstructProofFromRequest(evidenceType string, support *certprotos.EvidencePackage, purpose string) (*certprotos.VseClause, *certprotos.Proof, *certprotos.ProvedStatements) {

        // Debug
        fmt.Printf("\nConstructProofFromRequest\n")
        fmt.Printf("Submitted evidence type: %s\n", evidenceType)

        if support == nil {
                fmt.Printf("Empty support\n")
                return nil, nil, nil
        }

        if support.ProverType == nil {
                fmt.Printf("No prover type\n")
                return nil, nil, nil
        }

        if support.GetProverType() != "vse-verifier" {
                fmt.Printf("Only vse verifier supported\n")
                return nil, nil, nil
        }

        alreadyProved := &certprotos.ProvedStatements{}
        var toProve *certprotos.VseClause = nil
        var proof *certprotos.Proof = nil

        // Debug
        fmt.Printf("%d fact assertions in evidence\n", len(support.FactAssertion))
        for i := 0; i < len(support.FactAssertion); i++ {
                fmt.Printf("Type: %s\n",  support.FactAssertion[i].GetEvidenceType())
                if support.FactAssertion[i].GetEvidenceType() == "signed-claim" {
                        var sc certprotos.SignedClaimMessage
                        err := proto.Unmarshal(support.FactAssertion[i].SerializedEvidence, &sc)
                        if err != nil {
                                fmt.Printf("Can't unmarshal\n");
                        } else {
                        fmt.Printf("Clause: ")
                        vse:= certlib.GetVseFromSignedClaim(&sc)
                        certlib.PrintVseClause(vse)
                        }
                        fmt.Println("")
                } else if support.FactAssertion[i].GetEvidenceType() == "signed-vse-attestation-report" {
                        fmt.Printf("Signed report\n")
                } else if support.FactAssertion[i].GetEvidenceType() == "cert" {
                        fmt.Printf("Cert\n")
                } else if support.FactAssertion[i].GetEvidenceType() == "oe-attestation-report" {
                        fmt.Printf("oe-attestation-report\n")
                } else if support.FactAssertion[i].GetEvidenceType() == "gramine-attestation-report" {
                        fmt.Printf("gramine-attestation-report\n")
                } else if support.FactAssertion[i].GetEvidenceType() == "sev-attestation" {
                        fmt.Printf("sev-attestation\n")
                } else if support.FactAssertion[i].GetEvidenceType() == "pem-cert-chain" {
                        fmt.Printf("pem-cert-chain\n")
                } else {
                        fmt.Printf("Invalid evidence type\n")
                        return nil, nil, nil
                }
        }

        if !certlib.InitProvedStatements(*publicPolicyKey, support.FactAssertion, alreadyProved) {
                fmt.Printf("certlib.InitProvedStatements failed\n")
                return nil, nil, nil
        }

        // Debug
        fmt.Printf("\nInitial proved statements %d\n", len(alreadyProved.Proved))
        for i := 0; i < len(alreadyProved.Proved); i++ {
                certlib.PrintVseClause(alreadyProved.Proved[i])
                fmt.Println("")
        }
        fmt.Println("")

        // evidenceType should be "full-vse-support", "platform-attestation-only" or
        //      "oe-evidence" or "gramin-evidence" or "sev-platform-attestation-only"
        if evidenceType == "full-vse-support" {
        } else if evidenceType == "platform-attestation-only" {
                if !AddNewFactsForAbbreviatedPlatformAttestation(publicPolicyKey, alreadyProved) {
                        fmt.Printf("AddNewFactsForAbbreviatedPlatformAttestation failed\n")
                        return nil, nil, nil
                }
        } else if evidenceType == "sev-evidence" {
                if !AddNewFactsForSevEvidence(publicPolicyKey, alreadyProved) {
                        fmt.Printf("AddNewFactsForSevAttestation failed\n")
                        return nil, nil, nil
                }
        } else if evidenceType == "augmented-platform-attestation-only" {
                if !AddNewFactsForAugmentedPlatformAttestation(publicPolicyKey, alreadyProved) {
                        fmt.Printf("AddNewFactsForAugmentedPlatformAttestation failed\n")
                        return nil, nil, nil
                }
        } else if evidenceType == "oe-evidence" {
                if !AddNewFactsForOePlatformAttestation(publicPolicyKey, alreadyProved) {
                        fmt.Printf("AddNewFactsForOePlatformAttestation failed\n")
                        return nil, nil, nil
                }
        } else if evidenceType == "gramine-evidence" {
                if !AddNewFactsForGraminePlatformAttestation(publicPolicyKey, alreadyProved) {
                        fmt.Printf("AddNewFactsForGraminePlatformAttestation failed\n")
                        return nil, nil, nil
                }
        } else if evidenceType == "sev-platform-attestation-only" {
                if !AddNewFactsForSevEvidence(publicPolicyKey, alreadyProved) {
                        fmt.Printf("AddNewFactsForSevEvidence failed\n")
                        return nil, nil, nil
                }
        } else {
                fmt.Printf("Invalid Evidence type: %s\n", evidenceType)
                return nil, nil, nil
        }

        // Debug
        fmt.Printf("Augmented proved statements %d\n", len(alreadyProved.Proved))
        for i := 0; i < len(alreadyProved.Proved); i++ {
                certlib.PrintVseClause(alreadyProved.Proved[i])
                fmt.Println("")
        }

        if evidenceType == "full-vse-support" || evidenceType == "platform-attestation-only" {
                toProve, proof = ConstructProofFromFullVseEvidence(publicPolicyKey, purpose, *alreadyProved)
                if toProve == nil {
                        fmt.Printf("ConstructProofFromFullVseEvidence failed\n")
                        return nil, nil, nil
                }
        } else if evidenceType == "augmented-platform-attestation-only" {
                toProve, proof = ConstructProofFromShortVseEvidence(publicPolicyKey, purpose, *alreadyProved)
                if toProve == nil {
                        fmt.Printf("ConstructProofFromFullVseEvidence failed\n")
                        return nil, nil, nil
                }
        } else if evidenceType == "sev-platform-attestation-only" {
                toProve, proof = ConstructProofFromSevEvidence(publicPolicyKey, purpose, *alreadyProved)
                if toProve == nil {
                        fmt.Printf("ConstructProofFromSevEvidence failed\n")
                        return nil, nil, nil
                }
        } else if evidenceType == "oe-evidence" {
                toProve, proof = ConstructProofFromOeEvidence(publicPolicyKey, purpose, *alreadyProved)
                if toProve == nil {
                        fmt.Printf("ConstructProofFromOeEvidence failed\n")
                        return nil, nil, nil
                }
        } else if evidenceType == "gramine-evidence" {
                toProve, proof = ConstructProofFromGramineEvidence(publicPolicyKey, purpose, *alreadyProved)
                if toProve == nil {
                        fmt.Printf("ConstructProofFromGramineEvidence failed\n")
                        return nil, nil, nil
                }
        } else {
                return nil, nil, nil
        }

        // Debug
        if toProve != nil {
                fmt.Printf("To prove: ")
                certlib.PrintVseClause(toProve)
        }
        fmt.Printf("\n\nProof:\n")
        for i := 0; i < len(proof.Steps); i++ {
                certlib.PrintProofStep("    ", proof.Steps[i])
        }
        fmt.Println()
        fmt.Println()

        return toProve, proof, alreadyProved
}

func getAppMeasurementFromProvedStatements(appKeyEntity *certprotos.EntityMessage,
                alreadyProved *certprotos.ProvedStatements) []byte {

        for i := 0; i < len(alreadyProved.Proved); i++ {
                if certlib.SameEntity(alreadyProved.Proved[i].GetSubject(), appKeyEntity) {
                        if alreadyProved.Proved[i].GetVerb() == "speaks-for" {
                                if alreadyProved.Proved[i].Object != nil &&
                                        alreadyProved.Proved[i].Object.GetEntityType() == "measurement" {
                                        return alreadyProved.Proved[i].Object.Measurement
                                }
                        }
                }
        }
        return nil
}

func logRequest(b []byte) *string {
        if b == nil {
                return nil
        }
        s := strconv.Itoa(dataPacketFileNum)
        dataPacketFileNum = dataPacketFileNum + 1
        fileName := *logDir + "/" + "SSReq" + "-" + s
        if ioutil.WriteFile(fileName, b, 0666)  != nil {
                fmt.Printf("Can't write %s\n", fileName)
                return nil
        }
        return &fileName
}

func logResponse(b []byte) *string {
        if b == nil {
                return nil
        }
        s := strconv.Itoa(dataPacketFileNum)
        dataPacketFileNum = dataPacketFileNum + 1
        fileName := *logDir + "/" + "SSRsp" + "-" + s
        if ioutil.WriteFile(fileName, b, 0666)  != nil {
                fmt.Printf("Can't write %s\n", fileName)
                return nil
        }
        return &fileName
}

// Todo: Consider logging the proof and IP address too.
func logEvent(msg string, req []byte, resp []byte) {
        if !logging {
                return
        }
        reqName := logRequest(req)
        respName := logResponse(resp)
        logger.Printf("%s, ", msg)
        if reqName != nil {
                logger.Printf("%s ,", reqName)
        } else {
                logger.Printf("No request,")
        }
        if respName != nil {
                logger.Printf("%s\n", respName)
        } else {
                logger.Printf("No response\n")
        }
}

// Procedure is:
//      read a message
//      evaluate the trust assertion
//      if it succeeds,
//            sign a cert
//            save the proof, action and cert info in the transaction files
//            save net infor for forensics
//      if it fails
//            save net infor for forensics
//      if logging is enabled, log event, request and response
func serviceThread(conn net.Conn, client string) {

	b := certlib.SizedSocketRead(conn)
	if b == nil {
                logEvent("Can't read request", nil, nil)
                return
	}

        request:= &certprotos.TrustRequestMessage{}
        err := proto.Unmarshal(b, request)
        if err != nil {
                fmt.Println("serviceThread: Failed to decode request", err)
                logEvent("Can't unmarshal request", nil, nil)
                return
        }

        // Debug
        fmt.Printf("serviceThread: Trust request received:\n")
        certlib.PrintTrustRequest(request)

        // Prepare response
        succeeded := "succeeded"
        failed := "failed"

        response := certprotos.TrustResponseMessage{}
        response.RequestingEnclaveTag = request.RequestingEnclaveTag
        response.ProvidingEnclaveTag = request.ProvidingEnclaveTag
        response.Status = &failed

        // Construct the proof
        var purpose string
        if  request.Purpose == nil {
                purpose =  "authentication"
        } else {
                purpose =  *request.Purpose
        }
        toProve, proof, alreadyProved := ConstructProofFromRequest(
                        request.GetSubmittedEvidenceType(), request.GetSupport(),
                        purpose)
        if toProve == nil || proof == nil || alreadyProved == nil {
                // Debug
                fmt.Printf("Constructing Proof fails\n")
                logEvent("Can't construct proof from request", b, nil)

                // Debug
                fmt.Printf("Sending response\n")
                certlib.PrintTrustReponse(&response)

                // send response
                rb, err := proto.Marshal(&response)
                if err != nil {
                        logEvent("Couldn't marshall request", b, nil)
                        return
                }
		if !certlib.SizedSocketWrite(conn, rb) {
                        fmt.Printf("SizedSocketWrite failed\n")
                        return
		}
                if response.Status != nil && *response.Status == "succeeded" {
                        logEvent("Successful request", b, rb)
                } else {
                        logEvent("Failed Request", b, rb)
                }
                        return
        } else {
                // Debug
                fmt.Printf("Constructing Proof succeeded\n")
        }
        appKeyEntity := toProve.GetSubject()

        // Debug
        if toProve != nil {
                fmt.Printf("To prove: ")
                certlib.PrintVseClause(toProve)
                fmt.Printf("\n")
        }

        // Verify proof and send response
        var appOrgName string = "anonymous"
        if  toProve.Subject.Key != nil && toProve.Subject.Key.KeyName != nil {
                appOrgName = *toProve.Subject.Key.KeyName
        }

        // Debug
        fmt.Printf("Verifying proof %d steps\n", len(proof.Steps))

        // Check proof
        if proof == nil {
                response.Status = &failed
        } else if certlib.VerifyProof(publicPolicyKey, toProve, proof, alreadyProved) {
                fmt.Printf("Proof verified\n")
                // Produce Artifact
                if toProve.Subject == nil && toProve.Subject.Key == nil &&
                                toProve.Subject.Key.KeyName == nil {
                        fmt.Printf("toProve check failed\n")
                        certlib.PrintVseClause(toProve)
                        fmt.Println()
                        response.Status = &failed
                } else {
                        if purpose == "attestation" {
                                sr := certlib.ProducePlatformRule(&privatePolicyKey, policyCert,
                                        toProve.Subject.Key, duration)
                                if sr == nil {
                                        response.Status = &succeeded
                                } else {
                                        response.Status = &succeeded
                                        response.Artifact = sr
                                }
                        } else {
                                // find statement appKey speaks-for measurement in alreadyProved and reset appOrgName
                                m := getAppMeasurementFromProvedStatements(appKeyEntity,  alreadyProved)
                                if m != nil {
                                        appOrgName = "Measured-" + hex.EncodeToString(m)
                                }
                                sn = sn + 1
                                org := "CertifierUsers"
                                cert := certlib.ProduceAdmissionCert(&privatePolicyKey, policyCert,
                                        toProve.Subject.Key, org,
                                        appOrgName, sn, duration)
                                if cert == nil {
                                        fmt.Printf("certlib.ProduceAdmissionCert returned nil\n")
                                        response.Status = &failed
                                } else {
                                        response.Status = &succeeded
                                        response.Artifact = cert.Raw
                                }
                        }
                }
        } else {
                fmt.Printf("Verifying proof failed\n")
                response.Status = &failed
        }

	// TODO: REMOVE
	response.Status = &succeeded
	// TODO: REMOVE

        // Debug
        fmt.Printf("Sending response\n")
        certlib.PrintTrustReponse(&response)

        // send response
        rb, err := proto.Marshal(&response)
        if err != nil {
                logEvent("Couldn't marshall request", b, nil)
                return
        }
	if !certlib.SizedSocketWrite(conn, rb) {
                fmt.Printf("SizedSocketWrite failed (2)\n")
                return
	}
        if response.Status != nil && *response.Status == "succeeded" {
                logEvent("Successful request", b, rb)
        } else {
                logEvent("Failed Request", b, rb)
        }
        return
}


func server(serverAddr string, arg string) {

        if initCertifierService() != true {
                fmt.Println("Server: failed to initialize server")
                os.Exit(1)
        }

        var sock net.Listener
        var err error
        var conn net.Conn

        // Listen for clients.
        fmt.Printf("simpleserver: Listening\n")
        sock, err = net.Listen("tcp", serverAddr)
        if err != nil {
                fmt.Printf("simpleserver, listen error: ", err, "\n")
                return
        }

        // Service client connections.
        for {
                fmt.Printf("server: at accept\n")
                conn, err = sock.Accept()
                if err != nil {
                        fmt.Printf("simpleserver: can't accept connection: %s\n", err.Error())
                        continue
                }
                // Todo: maybe get client name and client IP for logging.
                var clientName string = "blah"
                go serviceThread(conn, clientName)
        }
}

func main() {

        flag.Parse()

        var serverAddr string
        serverAddr = *serverHost + ":" + *serverPort
        var arg string = "something"

        // later this may turn into a TLS connection, we'll see
        server(serverAddr, arg)
        fmt.Printf("simpleserver: done\n")
}
