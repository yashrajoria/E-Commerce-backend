Role creation and attach instructions

1. Edit placeholders in `trust.json` and `tf-policy.json`:
   - Replace `ACCOUNT_ID` with your AWS account id.
   - Replace `OWNER/REPO` in `trust.json` with your GitHub repo (e.g. `yashrajoria/E-Commerce-backend`).
   - If you use different resource names, update `tf-policy.json` accordingly.

2. Create role and attach inline policy (example):

```bash
# create role with trust policy
aws iam create-role \
  --role-name github-actions-terraform \
  --assume-role-policy-document file://infrastructure/aws/trust.json

# attach inline policy
aws iam put-role-policy \
  --role-name github-actions-terraform \
  --policy-name TerraformMinimalPolicy \
  --policy-document file://infrastructure/aws/tf-policy.json

# get role ARN (copy to GitHub secret AWS_ROLE_TO_ASSUME)
aws iam get-role --role-name github-actions-terraform --query 'Role.Arn' --output text
```

3. (Optional) Create a managed policy and attach instead:

```bash
aws iam create-policy --policy-name TerraformMinimalPolicy --policy-document file://infrastructure/aws/tf-policy.json
POLICY_ARN="arn:aws:iam::ACCOUNT_ID:policy/TerraformMinimalPolicy"
aws iam attach-role-policy --role-name github-actions-terraform --policy-arn "$POLICY_ARN"
```

4. Add the returned role ARN as GitHub secret `AWS_ROLE_TO_ASSUME`.

5. Trigger the terraform workflow (open a PR) to run `terraform plan` via CI. Review plan and apply in staging.

Security notes

- Use OIDC where possible; restrict the `sub` condition in `trust.json` to a specific ref or environment for tighter security.
- Keep `tf-policy.json` out of public repos when filled with real ARNs.
