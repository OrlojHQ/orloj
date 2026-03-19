import { FormEvent, useState } from "react";
import { setupLocalAuth } from "../api/client";

interface SetupProps {
  onSuccess: () => void;
}

export function Setup({ onSuccess }: SetupProps) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (password !== confirmPassword) {
      setError("Passwords do not match");
      return;
    }
    setSubmitting(true);
    setError("");
    try {
      await setupLocalAuth(username, password);
      onSuccess();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Setup failed");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="page">
      <div className="page__header">
        <h1 className="page__title">Initial Admin Setup</h1>
        <p className="page__subtitle">Create the local admin account to secure this installation.</p>
      </div>
      <div className="card" style={{ maxWidth: 520 }}>
        <form onSubmit={onSubmit} className="stack" style={{ gap: 12 }}>
          <label className="topbar__settings-label">
            Username
            <input
              autoFocus
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              autoComplete="username"
              required
            />
          </label>
          <label className="topbar__settings-label">
            Password (min 12 chars)
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="new-password"
              required
              minLength={12}
            />
          </label>
          <label className="topbar__settings-label">
            Confirm Password
            <input
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              autoComplete="new-password"
              required
              minLength={12}
            />
          </label>
          {error && <p className="text-red">{error}</p>}
          <button className="btn-primary" type="submit" disabled={submitting}>
            {submitting ? "Creating..." : "Create Admin"}
          </button>
        </form>
      </div>
    </div>
  );
}
