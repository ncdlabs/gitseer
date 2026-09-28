import { useState, type InputHTMLAttributes } from "react";
import { Glyph } from "./Glyph";

type Props = {
  id: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  autoComplete?: string;
  disabled?: boolean;
  required?: boolean;
  readOnly?: boolean;
  "aria-label"?: string;
  "aria-invalid"?: InputHTMLAttributes<HTMLInputElement>["aria-invalid"];
  "aria-describedby"?: string;
};

/** Chrome expects a username field beside password inputs (may be visually hidden). */
export function HiddenUsernameField({
  id = "password-form-username",
  value = "bootstrap",
}: {
  id?: string;
  value?: string;
}) {
  return (
    <input
      id={id}
      type="text"
      name="username"
      autoComplete="username"
      value={value}
      readOnly
      tabIndex={-1}
      className="visually-hidden"
      aria-hidden="true"
    />
  );
}

export function PasswordInput({
  id,
  value,
  onChange,
  placeholder,
  autoComplete,
  disabled,
  required,
  readOnly,
  "aria-label": ariaLabel,
  "aria-invalid": ariaInvalid,
  "aria-describedby": ariaDescribedBy,
}: Props) {
  const [visible, setVisible] = useState(false);

  return (
    <div className="password-input">
      <input
        id={id}
        type={visible ? "text" : "password"}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        autoComplete={autoComplete}
        disabled={disabled}
        required={required}
        readOnly={readOnly}
        aria-label={ariaLabel}
        aria-invalid={ariaInvalid}
        aria-describedby={ariaDescribedBy}
      />
      <button
        type="button"
        className="password-input__toggle"
        aria-label={visible ? "Hide password" : "Show password"}
        aria-pressed={visible}
        disabled={disabled}
        onClick={() => setVisible((v) => !v)}
      >
        <Glyph name={visible ? "eyeOff" : "eye"} className="password-input__icon" />
      </button>
    </div>
  );
}
