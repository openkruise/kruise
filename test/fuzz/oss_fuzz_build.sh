#!/bin/bash -eu
# Copyright 2025 The Kruise Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
set -o nounset
set -o pipefail
set -o errexit
set -x

go clean --modcache
go mod tidy
go mod vendor

rm -r $SRC/kruise/vendor
# Install the fuzzing library required for building fuzzers.
go get github.com/AdamKorcz/go-118-fuzz-build/testing

# Compile fuzzers for the WorkloadSpread component.
compile_native_go_fuzzer $SRC/kruise/pkg/controller/workloadspread FuzzPatchFavoriteSubsetMetadataToPod fuzz_patch_favorite_subset_metadata_to_pod
compile_native_go_fuzzer $SRC/kruise/pkg/controller/workloadspread FuzzPodPreferredScore fuzz_pod_preferred_score
compile_native_go_fuzzer $SRC/kruise/pkg/util/workloadspread FuzzInjectWorkloadSpreadIntoPod fuzz_inject_workloadspread_into_pod
compile_native_go_fuzzer $SRC/kruise/pkg/util/workloadspread FuzzNestedField fuzz_nested_field
compile_native_go_fuzzer $SRC/kruise/pkg/webhook/workloadspread/validating FuzzValidateWorkloadSpreadSpec fuzz_validate_workloadspread_spec
compile_native_go_fuzzer $SRC/kruise/pkg/webhook/workloadspread/validating FuzzValidateWorkloadSpreadConflict fuzz_validate_workloadspread_conflict
compile_native_go_fuzzer $SRC/kruise/pkg/webhook/workloadspread/validating FuzzValidateWorkloadSpreadTargetRefUpdate fuzz_validate_workloadspread_target_ref_update

# Compile fuzzers for the UnitedDeployment component.
compile_native_go_fuzzer $SRC/kruise/pkg/controller/uniteddeployment FuzzParseSubsetReplicas fuzz_parse_subset_replicas
compile_native_go_fuzzer $SRC/kruise/pkg/controller/uniteddeployment FuzzApplySubsetTemplate fuzz_apply_subset_template
compile_native_go_fuzzer $SRC/kruise/pkg/webhook/uniteddeployment/validating FuzzValidateUnitedDeploymentSpec fuzz_validate_uniteddeployment_spec

# Compile fuzzers for the ResourceDistribution component.
compile_native_go_fuzzer $SRC/kruise/pkg/controller/resourcedistribution FuzzMatchFunctions fuzz_match_functions
compile_native_go_fuzzer $SRC/kruise/pkg/controller/resourcedistribution FuzzMergeMetadata fuzz_merge_metadata
compile_native_go_fuzzer $SRC/kruise/pkg/webhook/resourcedistribution/validating FuzzDeserializeResource fuzz_deserialize_resource
compile_native_go_fuzzer $SRC/kruise/pkg/webhook/resourcedistribution/validating FuzzValidateResourceDistributionSpec fuzz_validate_resource_distribution_spec
compile_native_go_fuzzer $SRC/kruise/pkg/webhook/resourcedistribution/validating FuzzValidateResourceDistributionTargets fuzz_validate_resource_distribution_targets
compile_native_go_fuzzer $SRC/kruise/pkg/webhook/resourcedistribution/validating FuzzValidateResourceDistributionResource fuzz_validate_resource_distribution_resource

# Compile fuzzers for the SidecarSet component.
compile_native_go_fuzzer $SRC/kruise/pkg/control/sidecarcontrol FuzzPatchPodMetadata fuzz_patch_pod_metadata
compile_native_go_fuzzer $SRC/kruise/pkg/webhook/sidecarset/validating FuzzValidateSidecarSetSpec fuzz_validate_sidecarset_spec
compile_native_go_fuzzer $SRC/kruise/pkg/webhook/pod/mutating FuzzSidecarSetMutatingPod fuzz_sidecarset_mutating_pod

# Compile fuzzers for the BroadcastJob component.
compile_native_go_fuzzer $SRC/kruise/pkg/webhook/broadcastjob/validating FuzzValidateBroadcastJob fuzz_validate_broadcastjob
compile_native_go_fuzzer $SRC/kruise/pkg/webhook/broadcastjob/validating FuzzBroadcastJobSpecValidation fuzz_broadcastjob_spec_validation

# Compile fuzzers for the AdvancedCronJob component.
compile_native_go_fuzzer $SRC/kruise/pkg/webhook/advancedcronjob/validating FuzzValidateAdvancedCronJob fuzz_validate_advancedcronjob
compile_native_go_fuzzer $SRC/kruise/pkg/webhook/advancedcronjob/validating FuzzValidateAdvancedCronJobSpec fuzz_validate_advancedcronjob_spec
compile_native_go_fuzzer $SRC/kruise/pkg/webhook/advancedcronjob/validating FuzzValidateAdvancedCronJobSpecSchedule fuzz_validate_advancedcronjob_spec_schedule
compile_native_go_fuzzer $SRC/kruise/pkg/webhook/advancedcronjob/validating FuzzValidateAdvancedCronJobSpecTemplate fuzz_validate_advancedcronjob_spec_template