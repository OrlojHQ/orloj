import { FormEvent, useState } from "react";
import { useNavigate } from "react-router-dom";
import { loginLocalAuth } from "../api/client";
import { AuthShell } from "../components/AuthShell";

interface LoginProps {
  onSuccess: () => void;
}

export function Login({ onSuccess }: LoginProps) {
  const navigate = useNavigate();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setSubmitting(true);
    setError("");
    try {
      await loginLocalAuth(username, password);
      onSuccess();
      navigate("/", { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <AuthShell
      mode="login"
      title="Sign in to continue"
      subtitle="Use your local admin credentials to access this Orloj environment."
    >
      <form onSubmit={onSubmit} className="auth-form">
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
            autoComplete="current-password"
            required
          />
        </label>
        {error && <p className="auth-form__error">{error}</p>}
        <button className="btn-primary auth-form__submit" type="submit" disabled={submitting}>
          {submitting ? "Signing in..." : "Sign In"}
        </button>
      </form>
    </AuthShell>
  );
}
