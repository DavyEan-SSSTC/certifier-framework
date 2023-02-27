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
#include <iostream>
#include "gramine_api.h"

bool gramine_local_certify();
bool gramine_seal();
bool gramine_setup_certifier_functions(GramineCertifierFunctions gramineFuncs);
bool certifier_init(char* usr_data_dir, size_t usr_data_dir_size);
bool cold_init();
bool certify_me();
bool setup_server_ssl();
bool setup_client_ssl();
