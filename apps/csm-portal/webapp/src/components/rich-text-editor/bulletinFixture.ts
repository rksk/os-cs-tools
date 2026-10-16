// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

// Test fixture only: not imported by any production module.
/** The shape of the ServiceNow-era security bulletin: a styled box holding one row per advisory. */
export const BULLETIN_HTML = `<p>Kindly refer to the <strong>Security Advisories</strong> section below.</p>
<p>Please note that only versions affected according to our<a style="color: #ff7300 !important; text-decoration: none;" href="https://example.test/policy" target="_blank" rel="noopener noreferrer"> back porting policy</a> are included.</p>
<p><strong>Security Advisories</strong></p>
<div style="width: 100%;box-sizing: border-box;padding: 15px;border-radius: 12px;border: 1px solid rgba(128,128,128,0.25);background: linear-gradient(160deg, rgba(255,255,255,0.05) 0%, rgba(255,255,255,0) 60%), rgba(128,128,128,0.06);box-shadow: 0 2px 8px rgba(0,0,0,0.12);">
  <div style="margin: 0;">
    <div><span><strong>WSO2-2026-0001: Security Advisory for WSO2 Identity Server</strong></span></div>
    <div>Severity: Medium</div>
    <div><a style="color: #1a8038 !important; text-decoration: underline;" href="https://example.test/a.pdf">Download here</a></div>
  </div>
</div>`;
