# Coding Standards Adoption

## 1. Action Taken
We have reviewed the entirety of the `/docs/style` directory in the `ChatProjectsToKraxBridge` workspace, as well as the newly instated `rust_style.md` convention in the `ChatProjects` workspace. We are officially adopting the **Rust Coding Style & Commenting Conventions** as the law for the development of this bridge.

## 2. Why We Did This
The underlying philosophy across all the project's style guides (Go, Python, PHP, JS, and Rust) is: **Clarity > Brevity > Cleverness**.
By enforcing these rules up front, we guarantee that the codebase will be readable not just by the author today, but by human engineers and autonomous agents (like Kaelen or Mason) months from now.

## 3. Strict Rules Enforced
For all Rust code written in the `ChatProjectsToKraxBridge` project going forward, we will strictly enforce:
1. **Explicit Naming:** No single-letter variable names. Variables will be named descriptively (e.g., `extracted_feature_payload` instead of `ext_pay` or `e`).
2. **Dense Commenting:** A comment explaining *intent* will be placed every 2-4 logical lines. Complex operations will have block comments describing the algorithm phase.
3. **Explicit Types:** We will not rely heavily on Rust's type inference. Public API boundaries and non-trivial structs will have explicit types defined for immediate readability.
4. **Deterministic and Guarded Flow:** Heavy use of guard clauses with explicit failure messages (`expect()` instead of raw `unwrap()`). No silent failures.
