#
#   File: src/cca/islet_test/Makefile
#
GPP=g++
ISLET_PATH = ../../../third_party/islet
ISLET_INCLUDE= -I$(ISLET_PATH)/include
ISLET_LDFLAGS= -L$(ISLET_PATH)/lib -lislet_sdk

CERTIFIER_INCLUDE= -I../../../include

.PHONY: all
all: attest_seal_test

CFLAGS += $(ISLET_INCLUDE) $(CERTIFIER_INCLUDE)
LDFLAGS += $(ISLET_LDFLAGS) -lprotobuf -lcrypto -lssl

.PHONY: attest_seal_test
attest_seal_test: attest_seal_test.cc
	@echo "Compiling $<"
	@$(GPP) $< $(CFLAGS) $(LDFLAGS) -o $@
	@LD_LIBRARY_PATH=$(ISLET_PATH)/lib ./$@

.PHONY: shim_test
shim_test: shim_test.cc ../cca_shim.cc
	@echo "Compiling $<"
	@$(GPP) $^ $(CFLAGS) ../../../src/support.o ../../certifier.pb.o $(LDFLAGS) -o $@
	@LD_LIBRARY_PATH=$(ISLET_PATH)/lib ./$@

.PHONY: test
test: attest_seal_test shim_test

.PHONY: clean
clean:
	@echo "removing executable files"
	@rm -rf attest_seal_test
