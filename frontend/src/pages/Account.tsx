import { FormEvent, useState } from "react";
import { useNavigate } from "react-router-dom";
import { changeLocalAuthPassword, logoutLocalAuth } from "../api/client";
import { toast } from "../components/Toast";

interface AccountPageProps {
  authMode: string;
  authMethod?: string;
  username?: string;
  onAuthStateChanged: () => void;
}

export function AccountPage({ authMode, authMethod, username, onAuthStateChanged }: AccountPageProps) {
  const navigate = useNavigate();
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [signingOut, setSigningOut] = useState(false);
  const [error, setError] = useState("");

  async function handlePasswordSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (newPassword !== confirmPassword) {
      setError("New password and confirmation do not match");
      return;
    }
    if (newPassword.length < 12) {
      setError("New password must be at least 12 characters");
      return;
    }

    setSubmitting(true);
    setError("");
    try {
      await changeLocalAuthPassword(currentPassword, newPassword);
      toast("success", "Password changed. Please sign in again.");
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      onAuthStateChanged();
      navigate("/", { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to change password");
    } finally {
      setSubmitting(false);
    }
  }

  async function handleSignOut() {
    setSigningOut(true);
    try {
      await logoutLocalAuth();
      toast("info", "Signed out");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to sign out");
    } finally {
      setSigningOut(false);
      onAuthStateChanged();
      navigate("/", { replace: true });
    }
  }

  return (
    <div className="page account-page">
      <div className="page__header">
        <div>
          <h1 className="page__title">Account Settings</h1>
          <p className="page__subtitle">Manage local authentication and session controls.</p>
        </div>
      </div>

      <div className="account-grid">
        <section className="card account-section">
          <h2 className="account-section__title">Profile</h2>
          <div className="detail-grid">
            <div className="detail-field">
              <span className="detail-field__label">Username</span>
              <span className="detail-field__value mono">{username?.trim() || "local-admin"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Auth Mode</span>
              <span className="detail-field__value">{authMode || "native"}</span>
            </div>
            <div className="detail-field">
              <span className="detail-field__label">Session Method</span>
              <span className="detail-field__value">{authMethod || "session"}</span>
            </div>
            <div className="detail-field detail-field--full">
              <span className="detail-field__label">Username Changes</span>
              <span className="detail-field__value">
                Username editing is CLI-only for now. Password changes are available below.
              </span>
            </div>
          </div>
        </section>

        <section className="card account-section">
          <h2 className="account-section__title">Security</h2>
          <form onSubmit={handlePasswordSubmit} className="auth-form account-form">
            <label className="auth-form__field">
              <span className="auth-form__label">Current Password</span>
              <input
                type="password"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
                autoComplete="current-password"
                required
              />
            </label>
            <label className="auth-form__field">
              <span className="auth-form__label">New Password</span>
              <input
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                autoComplete="new-password"
                minLength={12}
                required
              />
              <span className="auth-form__hint">Minimum length: 12 characters.</span>
            </label>
            <label className="auth-form__field">
              <span className="auth-form__label">Confirm New Password</span>
              <input
                type="password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                autoComplete="new-password"
                minLength={12}
                required
              />
            </label>
            {error && <p className="auth-form__error">{error}</p>}
            <button className="btn-primary account-form__submit" type="submit" disabled={submitting}>
              {submitting ? "Updating password..." : "Change Password"}
            </button>
          </form>
        </section>

        <section className="card account-section account-section--session">
          <h2 className="account-section__title">Session</h2>
          <p className="text-muted text-xs account-section__copy">
            Sign out this browser session if you are done or if credentials changed.
          </p>
          <button type="button" className="btn-secondary" onClick={handleSignOut} disabled={signingOut}>
            {signingOut ? "Signing out..." : "Sign Out"}
          </button>
        </section>
      </div>
    </div>
  );
}
