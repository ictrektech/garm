// Copyright 2026 Cloudbase Solutions SRL
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package runner

import (
	"os"
	"strings"
)

// VOSGiteaPath is the path of Gitea on the VOS gateway.
const VOSGiteaPath = "/app/com.ictrek.gitea/"

// vosInternalBase is the plain HTTP gateway that services and runner
// instances on the device use. It must not be the browser facing origin:
// runner instances cannot verify the platform certificate.
const vosInternalBase = "http://172.17.0.1:1182"

// VOSInternalBaseURL returns the gateway URL of the device.
func VOSInternalBaseURL() string {
	if base := strings.TrimSpace(os.Getenv("GARM_VOS_INTERNAL_BASE")); base != "" {
		return strings.TrimRight(base, "/")
	}
	return vosInternalBase
}

// VOSGiteaURL returns the base URL of the Gitea running on the device.
func VOSGiteaURL() string {
	return VOSInternalBaseURL() + VOSGiteaPath
}
