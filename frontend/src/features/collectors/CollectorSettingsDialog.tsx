import { type SubmitEvent, useState } from 'react';
import {
  Button,
  Checkbox,
  FormControlLabel,
  Stack,
  Switch,
  TextField,
  Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import type {
  Collector,
  CollectorSetting,
  CollectorSettingsInput,
  SettingValue,
} from '../../api/types';
import ErrorBanner from '../../components/ErrorBanner';
import Modal from '../../components/Modal';
import { errorMessage } from '../../lib/errors';
import { localized } from '../../lib/localized';
import './CollectorSettingsDialog.scss';

/** What a field holds while editing: text for text and number fields, a flag
 * for switches. A secret's text is a new value; empty keeps the stored one. */
type Draft = Record<string, string | boolean>;

function initialDraft(c: Collector): Draft {
  const draft: Draft = {};
  for (const s of c.settingsSchema) {
    const value = c.settings[s.key];
    if (s.type === 'boolean') draft[s.key] = Boolean(value ?? s.default ?? false);
    else if (s.type === 'secret') draft[s.key] = '';
    else draft[s.key] = value === undefined ? '' : String(value);
  }
  return draft;
}

/** Turns the draft into the settings to send, or the key of the first field
 * that doesn't hold a valid number. */
function toInput(
  schema: CollectorSetting[],
  draft: Draft,
  clearSecrets: ReadonlySet<string>,
): { input: CollectorSettingsInput } | { invalid: string } {
  const input: CollectorSettingsInput = {};
  for (const s of schema) {
    const value = draft[s.key];
    if (typeof value === 'boolean') {
      input[s.key] = value;
      continue;
    }

    const text = value.trim();
    if (s.type === 'secret') {
      if (clearSecrets.has(s.key)) input[s.key] = null;
      else if (text) input[s.key] = text;
      continue;
    }

    if (!text) continue;
    if (s.type === 'integer' || s.type === 'number') {
      const n = Number(text);
      if (!Number.isFinite(n) || (s.type === 'integer' && !Number.isInteger(n))) {
        return { invalid: s.key };
      }
      input[s.key] = n;
    } else {
      input[s.key] = text;
    }
  }
  return { input };
}

function defaultHint(value: SettingValue | undefined): string | undefined {
  return value === undefined ? undefined : String(value);
}

function SettingField({
  setting,
  value,
  secretSet,
  clear,
  invalid,
  onChange,
  onClearChange,
}: {
  setting: CollectorSetting;
  value: string | boolean;
  secretSet: boolean;
  clear: boolean;
  invalid: boolean;
  onChange: (value: string | boolean) => void;
  onClearChange: (clear: boolean) => void;
}) {
  const { t, i18n } = useTranslation();
  const label = localized(setting.label, i18n.language);
  const help = localized(setting.description, i18n.language);

  if (setting.type === 'boolean') {
    return (
      <div>
        <FormControlLabel
          control={<Switch checked={value === true} onChange={(e) => onChange(e.target.checked)} />}
          label={label}
        />
        {help && (
          <Typography variant="caption" color="text.secondary" component="div">
            {help}
          </Typography>
        )}
      </div>
    );
  }

  const isSecret = setting.type === 'secret';
  const placeholder = isSecret
    ? secretSet
      ? t('collectors.secretKeep')
      : undefined
    : defaultHint(setting.default);

  return (
    <Stack spacing={0.5}>
      <TextField
        label={label}
        size="small"
        type={isSecret ? 'password' : 'text'}
        required={setting.required && !isSecret && setting.default === undefined}
        disabled={clear}
        error={invalid}
        helperText={
          invalid
            ? t(
                setting.type === 'integer'
                  ? 'collectors.invalidInteger'
                  : 'collectors.invalidNumber',
              )
            : help || undefined
        }
        placeholder={placeholder}
        autoComplete={isSecret ? 'new-password' : 'off'}
        slotProps={{
          htmlInput: { inputMode: setting.type === 'string' || isSecret ? 'text' : 'decimal' },
          inputLabel: placeholder ? { shrink: true } : undefined,
        }}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
      {isSecret && secretSet && (
        <FormControlLabel
          control={
            <Checkbox
              size="small"
              checked={clear}
              onChange={(e) => onClearChange(e.target.checked)}
            />
          }
          label={t('collectors.secretClear')}
        />
      )}
    </Stack>
  );
}

/** Edits a collector's settings, as its describe output declares them.
 * Give it a new `key` for each opening, so it starts from the stored values. */
export default function CollectorSettingsDialog({
  collector,
  onClose,
  onSave,
}: {
  collector: Collector | null;
  onClose: () => void;
  onSave: (id: string, input: CollectorSettingsInput) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<Draft>(() => (collector ? initialDraft(collector) : {}));
  const [clearSecrets, setClearSecrets] = useState<Set<string>>(new Set());
  const [invalid, setInvalid] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  if (!collector) return null;

  const handleSubmit = async (e: SubmitEvent<HTMLFormElement>) => {
    e.preventDefault();
    const result = toInput(collector.settingsSchema, draft, clearSecrets);
    if ('invalid' in result) {
      setInvalid(result.invalid);
      return;
    }
    setInvalid(null);
    setError(null);
    setSaving(true);
    try {
      await onSave(collector.id, result.input);
    } catch (err) {
      setError(errorMessage(err));
      setSaving(false);
    }
  };

  const setClear = (key: string, clear: boolean) => {
    const next = new Set(clearSecrets);
    if (clear) next.add(key);
    else next.delete(key);
    setClearSecrets(next);
  };

  return (
    <Modal
      open
      title={t('collectors.settingsTitle', { name: collector.name || collector.id })}
      onClose={onClose}
    >
      <Stack
        component="form"
        spacing={3}
        className="collector-settings-dialog__body"
        onSubmit={handleSubmit}
      >
        <ErrorBanner message={error} />
        {collector.settingsSchema.map((s) => (
          <SettingField
            key={s.key}
            setting={s}
            value={draft[s.key]}
            secretSet={collector.secretsSet.includes(s.key)}
            clear={clearSecrets.has(s.key)}
            invalid={invalid === s.key}
            onChange={(value) => setDraft({ ...draft, [s.key]: value })}
            onClearChange={(clear) => setClear(s.key, clear)}
          />
        ))}
        <Stack direction="row" spacing={1} className="collector-settings-dialog__actions">
          <Button onClick={onClose} disabled={saving}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" variant="contained" disabled={saving}>
            {t('common.save')}
          </Button>
        </Stack>
      </Stack>
    </Modal>
  );
}
