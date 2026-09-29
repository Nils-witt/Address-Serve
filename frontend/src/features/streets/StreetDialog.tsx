import { type SubmitEvent, useState } from 'react';
import { Autocomplete, Button, Stack, TextField } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Street, StreetInput } from '../../api/types';
import CoordinateFields from '../../components/CoordinateFields';
import ErrorBanner from '../../components/ErrorBanner';
import Modal from '../../components/Modal';
import { useCities, useDistricts } from '../../hooks/useStreets';
import { parseCoordinates } from '../../lib/coordinates';
import { errorMessage } from '../../lib/errors';
import './StreetDialog.scss';

/** Values a new street starts with, e.g. the city the list is filtered by. */
export type StreetDefaults = Partial<Pick<StreetInput, 'city' | 'district' | 'country'>>;

/** Mounted by the modal only while it is open, so it starts fresh each time. */
function StreetForm({
  street,
  defaults,
  onClose,
  onSubmit,
}: {
  street: Street | null;
  defaults: StreetDefaults;
  onClose: () => void;
  onSubmit: (input: StreetInput) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(street?.name ?? '');
  const [city, setCity] = useState(street?.city ?? defaults.city ?? '');
  const [district, setDistrict] = useState(street?.district ?? defaults.district ?? '');
  const [country, setCountry] = useState(street?.country ?? defaults.country ?? '');
  const [latitude, setLatitude] = useState(street ? String(street.latitude) : '');
  const [longitude, setLongitude] = useState(street ? String(street.longitude) : '');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const { data: cities } = useCities();
  const { data: districts } = useDistricts(city.trim());

  const handleSubmit = async (e: SubmitEvent<HTMLFormElement>) => {
    e.preventDefault();
    const coordinates = parseCoordinates(latitude, longitude);
    if (!coordinates) {
      setError(t('fields.invalidCoordinates'));
      return;
    }
    setError(null);
    setSubmitting(true);
    try {
      await onSubmit({
        name: name.trim(),
        city: city.trim(),
        district: district.trim(),
        country: country.trim(),
        ...coordinates,
      });
      onClose();
    } catch (err) {
      setError(errorMessage(err));
      setSubmitting(false);
    }
  };

  return (
    <Stack component="form" spacing={3} className="street-dialog__form" onSubmit={handleSubmit}>
      <ErrorBanner message={error} />
      <TextField
        label={t('fields.streetName')}
        size="small"
        required
        autoFocus
        value={name}
        onChange={(e) => setName(e.target.value)}
      />
      <Autocomplete
        freeSolo
        options={cities}
        inputValue={city}
        onInputChange={(_, value) => setCity(value)}
        renderInput={(params) => (
          <TextField {...params} label={t('fields.city')} size="small" required />
        )}
      />
      <Autocomplete
        freeSolo
        options={[...new Set(districts.map((d) => d.name))]}
        inputValue={district}
        onInputChange={(_, value) => setDistrict(value)}
        renderInput={(params) => (
          <TextField {...params} label={t('fields.district')} size="small" required />
        )}
      />
      <TextField
        label={t('fields.country')}
        size="small"
        required
        value={country}
        onChange={(e) => setCountry(e.target.value)}
      />
      <CoordinateFields
        latitude={latitude}
        longitude={longitude}
        onLatitudeChange={setLatitude}
        onLongitudeChange={setLongitude}
      />
      <Stack direction="row" spacing={1} className="street-dialog__actions">
        <Button onClick={onClose} disabled={submitting}>
          {t('common.cancel')}
        </Button>
        <Button type="submit" variant="contained" disabled={submitting}>
          {street ? t('common.save') : t('common.create')}
        </Button>
      </Stack>
    </Stack>
  );
}

/** Creates a street (`street` is null) or edits an existing one. */
export default function StreetDialog({
  open,
  street,
  defaults = {},
  onClose,
  onSubmit,
}: {
  open: boolean;
  street: Street | null;
  defaults?: StreetDefaults;
  onClose: () => void;
  onSubmit: (input: StreetInput) => Promise<void>;
}) {
  const { t } = useTranslation();
  return (
    <Modal
      open={open}
      title={
        street ? t('streetDialog.editTitle', { name: street.name }) : t('streetDialog.newTitle')
      }
      onClose={onClose}
    >
      <StreetForm street={street} defaults={defaults} onClose={onClose} onSubmit={onSubmit} />
    </Modal>
  );
}
