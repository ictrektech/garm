// Copyright 2025 Cloudbase Solutions SRL
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//    License for the specific language governing permissions and limitations
//    under the License.

package templates

import (
	"strings"
	"testing"

	commonParams "github.com/cloudbase/garm-provider-common/params"

	"github.com/cloudbase/garm/params"
)

// Every install script that configures the agent has to state the transport,
// otherwise the agent refuses a plain http controller and no runner ever starts.
func TestAgentScriptsStateTheTransport(t *testing.T) {
	for _, forge := range []params.EndpointType{params.GiteaEndpointType, params.GithubEndpointType} {
		for _, osType := range []commonParams.OSType{commonParams.Linux, commonParams.Windows} {
			data, err := GetTemplateContent(osType, forge)
			if err != nil {
				t.Fatalf("%s/%s: %v", forge, osType, err)
			}
			if !strings.Contains(string(data), "force_insecure") {
				t.Errorf("%s/%s: the agent configuration does not set force_insecure", forge, osType)
			}
			// Boot updates are off, so cloud-init no longer installs these.
			if osType != commonParams.Linux {
				continue
			}
			tools := []string{"command -v curl"}
			if forge == params.GithubEndpointType {
				tools = append(tools, "command -v tar") // the github runner is a tarball
			}
			for _, tool := range tools {
				if !strings.Contains(string(data), tool) {
					t.Errorf("%s/%s: the script does not check for %s", forge, osType, tool)
				}
			}
		}
	}
}
