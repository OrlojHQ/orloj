import { FormEvent, useState } from "react";
import { useNavigate } from "react-router-dom";
import { setupLocalAuth } from "../api/client";
import { AuthShell } from "../components/AuthShell";

interface SetupProps {
  onSuccess: () => void;
  /** From `GET /v1/auth/config` when `ORLOJ_SETUP_TOKEN` is configured on the server. */
  setupTokenRequired?: boolean;
}

export function Setup({ onSuccess, setupTokenRequired = false }: SetupProps) {
  const navigate = useNavigate();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [setupToken, setSetupToken] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (password !== confirmPassword) {
      setError("Passwords do not match");
      return;
    }
    if (setupTokenRequired && !setupToken.trim()) {
      setError("Setup token is required");
      return;
    }
    setSubmitting(true);
    setError("");
    try {
      await setupLocalAuth(username, password, setupTokenRequired ? setupToken : undefined);
      onSuccess();
      navigate("/", { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Setup failed");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <AuthShell
      mode="setup"
      title="Create local admin account"
      subtitle="Set up initial credentials to secure this installation."
    >
      <form onSubmit={onSubmit} className="auth-form">
        {setupTokenRequired && (
          <label className="auth-form__field">
            <span className="auth-form__label">Setup token</span>
            <input
              type="password"
              value={setupToken}
              onChange={(e) => setSetupToken(e.target.value)}
              autoComplete="off"
              required
            />
            <span className="auth-form__hint">
              Value of <code>ORLOJ_SETUP_TOKEN</code> on the server.
            </span>
          </label>
        )}
        <label className="auth-form__field">
          <span className="auth-form__label">Username</span>
          <input
            autoFocus
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="username"
            required
          />
        </label>
        <label className="auth-form__field">
          <span className="auth-form__label">Password</span>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="new-password"
            required
            minLength={12}
          />
          <span className="auth-form__hint">Must be at least 12 characters.</span>
        </label>
        <label className="auth-form__field">
          <span className="auth-form__label">Confirm Password</span>
          <input
            type="password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            autoComplete="new-password"
            required
            minLength={12}
          />
        </label>
        {error && <p className="auth-form__error">{error}</p>}
        <button className="btn-primary auth-form__submit" type="submit" disabled={submitting}>
          {submitting ? "Creating admin..." : "Create Admin"}
        </button>
      </form>
    </AuthShell>
  );
}
