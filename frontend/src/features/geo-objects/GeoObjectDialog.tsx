import { type SubmitEvent, useMemo, useState } from 'react';
import { Autocomplete, Button, MenuItem, Stack, TextField } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { GeoObject, GeoObjectInput, Street } from '../../api/types';
import CoordinateFields from '../../components/CoordinateFields';
import ErrorBanner from '../../components/ErrorBanner';
import Modal from '../../components/Modal';
import { useDebouncedValue } from '../../hooks/useDebouncedValue';
import { useGeoObjectGroups } from '../../hooks/useGeoObjects';
import { useHouseNumbers } from '../../hooks/useHouseNumbers';
import { useStreets } from '../../hooks/useStreets';
import { parseCoordinates } from '../../lib/coordinates';
import { errorMessage } from '../../lib/errors';
import { compareHouseNumbers, formatHouseNumber, formatStreet } from '../../lib/format';
import './GeoObjectDialog.scss';

/** Typed characters needed before streets are searched; fewer would list
 * nearly every street. */
const MIN_STREET_SEARCH = 2;

/** Picks a street by searching its name. */
function StreetPicker({
  value,
  onChange,
}: {
  value: Street | null;
  onChange: (street: Street | null) => void;
}) {
  const { t } = useTranslation();
  const [input, setInput] = useState(value ? formatStreet(value) : '');
  const search = useDebouncedValue(input.trim());
  // While the input shows the chosen street, there is nothing to search for.
  const searching =
    search.length >= MIN_STREET_SEARCH && (!value || search !== formatStreet(value));
  const filter = useMemo(() => ({ name: search }), [search]);
  const { data: found, loading } = useStreets(filter, searching);

  // The chosen street must stay among the options, or Autocomplete drops it.
  const options = useMemo(() => {
    const list = searching ? found : [];
    return value && !list.some((s) => s.id === value.id) ? [value, ...list] : list;
  }, [found, searching, value]);

  return (
    <Autocomplete
      options={options}
      value={value}
      onChange={(_, street) => onChange(street)}
      inputValue={input}
      onInputChange={(_, text) => setInput(text)}
      getOptionLabel={formatStreet}
      isOptionEqualToValue={(a, b) => a.id === b.id}
      // The server already filtered by name.
      filterOptions={(o) => o}
      loading={searching && loading}
      noOptionsText={
        input.trim().length < MIN_STREET_SEARCH
          ? t('geoObjectDialog.streetSearchHint')
          : t('streets.empty')
      }
      renderInput={(params) => (
        <TextField {...params} label={t('fields.streetName')} size="small" required />
      )}
    />
  );
}

/** Mounted by the modal only while it is open, so it starts fresh each time. */
function GeoObjectForm({
  geoObject,
  initialStreet,
  fixedStreet,
  onClose,
  onSubmit,
}: {
  geoObject: GeoObject | null;
  initialStreet: Street | null;
  fixedStreet: boolean;
  onClose: () => void;
  onSubmit: (input: GeoObjectInput) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(geoObject?.name ?? '');
  const [group, setGroup] = useState(geoObject?.group ?? '');
  const [street, setStreet] = useState<Street | null>(initialStreet);
  const [houseNumberId, setHouseNumberId] = useState(geoObject?.houseNumberId ?? '');
  const [latitude, setLatitude] = useState(geoObject ? String(geoObject.latitude) : '');
  const [longitude, setLongitude] = useState(geoObject ? String(geoObject.longitude) : '');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const groups = useGeoObjectGroups();
  const houseNumbers = useHouseNumbers(street?.id ?? '');
  const sortedHouseNumbers = useMemo(
    () => [...houseNumbers.data].sort(compareHouseNumbers),
    [houseNumbers.data],
  );

  const onStreetChange = (next: Street | null) => {
    setStreet(next);
    // A house number only belongs to its own street.
    setHouseNumberId('');
  };

  const onHouseNumberChange = (id: string) => {
    setHouseNumberId(id);
    // Start from the house number's position unless one was entered already.
    const h = houseNumbers.data.find((n) => n.id === id);
    if (h && !latitude.trim() && !longitude.trim()) {
      setLatitude(String(h.latitude));
      setLongitude(String(h.longitude));
    }
  };

  const handleSubmit = async (e: SubmitEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!street) {
      setError(t('geoObjectDialog.streetRequired'));
      return;
    }
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
        group: group.trim(),
        streetId: street.id,
        houseNumberId: houseNumberId || null,
        ...coordinates,
      });
      onClose();
    } catch (err) {
      setError(errorMessage(err));
      setSubmitting(false);
    }
  };

  return (
    <Stack component="form" spacing={3} className="geo-object-dialog__form" onSubmit={handleSubmit}>
      <ErrorBanner message={error} />
      <TextField
        label={t('fields.name')}
        size="small"
        required
        autoFocus
        value={name}
        onChange={(e) => setName(e.target.value)}
      />
      <Autocomplete
        freeSolo
        options={groups}
        inputValue={group}
        onInputChange={(_, value) => setGroup(value)}
        renderInput={(params) => (
          <TextField {...params} label={t('fields.group')} size="small" required />
        )}
      />
      {fixedStreet ? (
        <TextField
          label={t('fields.streetName')}
          size="small"
          disabled
          value={street ? formatStreet(street) : ''}
        />
      ) : (
        <StreetPicker value={street} onChange={onStreetChange} />
      )}
      <TextField
        select
        label={t('fields.houseNumber')}
        size="small"
        disabled={!street}
        value={houseNumberId}
        onChange={(e) => onHouseNumberChange(e.target.value)}
        helperText={t('geoObjectDialog.houseNumberHelp')}
      >
        <MenuItem value="">
          <em>{t('geoObjectDialog.noHouseNumber')}</em>
        </MenuItem>
        {/* Keeps the current value valid while the street's house numbers load. */}
        {houseNumberId && !sortedHouseNumbers.some((h) => h.id === houseNumberId) && (
          <MenuItem value={houseNumberId}>…</MenuItem>
        )}
        {sortedHouseNumbers.map((h) => (
          <MenuItem key={h.id} value={h.id}>
            {formatHouseNumber(h)}
          </MenuItem>
        ))}
      </TextField>
      {geoObject?.externalId && (
        <TextField
          label={t('geoObjectDialog.externalId')}
          size="small"
          disabled
          value={geoObject.externalId}
          helperText={t('geoObjectDialog.externalIdHelp')}
        />
      )}
      <CoordinateFields
        latitude={latitude}
        longitude={longitude}
        onLatitudeChange={setLatitude}
        onLongitudeChange={setLongitude}
      />
      <Stack direction="row" spacing={1} className="geo-object-dialog__actions">
        <Button onClick={onClose} disabled={submitting}>
          {t('common.cancel')}
        </Button>
        <Button type="submit" variant="contained" disabled={submitting}>
          {geoObject ? t('common.save') : t('common.create')}
        </Button>
      </Stack>
    </Stack>
  );
}

/** Creates a geo object (`geoObject` is null) or edits one. */
export default function GeoObjectDialog({
  open,
  geoObject,
  street,
  fixedStreet = false,
  onClose,
  onSubmit,
}: {
  open: boolean;
  geoObject: GeoObject | null;
  /** The geo object's street, or the one a new geo object starts on. */
  street: Street | null;
  /** Keeps the geo object on `street`, e.g. when opened from the street page. */
  fixedStreet?: boolean;
  onClose: () => void;
  onSubmit: (input: GeoObjectInput) => Promise<void>;
}) {
  const { t } = useTranslation();
  return (
    <Modal
      open={open}
      title={
        geoObject
          ? t('geoObjectDialog.editTitle', { name: geoObject.name })
          : t('geoObjectDialog.newTitle')
      }
      onClose={onClose}
    >
      <GeoObjectForm
        geoObject={geoObject}
        initialStreet={street}
        fixedStreet={fixedStreet}
        onClose={onClose}
        onSubmit={onSubmit}
      />
    </Modal>
  );
}
