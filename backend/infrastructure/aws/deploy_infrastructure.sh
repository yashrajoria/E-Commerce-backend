#!/usr/bin/env bash
set -euo pipefail

# deploy_infrastructure.sh
# Lightweight helper to init, plan and apply Terraform for this repo.
# Usage: ./deploy_infrastructure.sh [apply|plan]

WORKDIR="$(cd "$(dirname "$0")" && pwd)/terraform"

if ! command -v terraform >/dev/null 2>&1; then
  echo "terraform not found in PATH; please install Terraform v1.0+"
  exit 1
fi

if ! command -v aws >/dev/null 2>&1; then
  echo "aws CLI not found in PATH; please install AWS CLI v2"
  exit 1
fi

MODE=${1:-apply}

echo "Working directory: ${WORKDIR}"
pushd "${WORKDIR}" >/dev/null

echo "Initializing Terraform..."
terraform init -input=false

if [ "${MODE}" = "plan" ]; then
  echo "Planning Terraform..."
  terraform plan -out=plan.tfplan
  echo "Plan saved to plan.tfplan"
  popd >/dev/null
  exit 0
fi

if [ "${MODE}" = "apply" ]; then
  echo "Applying Terraform (auto-approve)..."
  terraform apply -auto-approve

  echo "Exporting outputs to JSON..."
  terraform output -json > ../terraform-outputs.json

  echo "Done. terraform outputs saved to infrastructure/aws/terraform-outputs.json"
  popd >/dev/null
  exit 0
fi

echo "Unknown mode: ${MODE}. Use 'plan' or 'apply'."
popd >/dev/null
exit 2
