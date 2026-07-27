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

// Change-request reference lookups (ServiceNow CMDB — groups, IT services, service offerings,
// configuration items). Drive the "More options" pickers on the change-request create form.
// Mirrors the webapp's BeGroup/BeItService/BeServiceOffering/BeConfigurationItem, trimmed to the
// fields the picker actually renders (id + a display name).

export interface GroupDto {
  id: string;
  name: string;
}

export interface GroupSearchResponseDto {
  groups: GroupDto[];
  total?: number;
  limit?: number;
  offset?: number;
}

export interface ItServiceDto {
  id: string;
  name?: string | null;
}

export interface ItServiceSearchResponseDto {
  services: ItServiceDto[];
  total?: number;
  limit?: number;
  offset?: number;
}

export interface ServiceOfferingDto {
  id: string;
  name: string;
}

export interface ServiceOfferingSearchResponseDto {
  serviceOfferings: ServiceOfferingDto[];
  total?: number;
  limit?: number;
  offset?: number;
}

export interface ConfigurationItemDto {
  id: string;
  name?: string | null;
}

export interface ConfigurationItemSearchResponseDto {
  configurationItems: ConfigurationItemDto[];
  total?: number;
  limit?: number;
  offset?: number;
}
