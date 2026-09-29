import { type SubmitEvent, useState } from 'react';
import { Button, Stack, TextField } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { HouseNumber, HouseNumberInput } from '../../api/types';
import CoordinateFields from '../../components/CoordinateFields';
import ErrorBanner from '../../components/ErrorBanner';
import Modal from '../../components/Modal';
import { parseCoordinates } from '../../lib/coordinates';
import { errorMessage } from '../../lib/errors';
import { formatHouseNumber } from '../../lib/format';
import './HouseNumberDialog.scss';

/** Mounted by the modal only while it is open, so it starts fresh each time. */
function HouseNumberForm({
  streetId,
  houseNumber,
  defaultPostcode,
  onClose,
  onSubmit,
}: {
  streetId: string;
  houseNumber: HouseNumber | null;
  defaultPostcode: string;
  onClose: () => void;
  onSubmit: (input: HouseNumberInput) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [number, setNumber] = useState(houseNumber ? String(houseNumber.number) : '');
  const [numberAddition, setNumberAddition] = useState(houseNumber?.numberAddition ?? '');
  const [postcode, setPostcode] = useState(houseNumber?.postcode ?? defaultPostcode);
  const [latitude, setLatitude] = useState(houseNumber ? String(houseNumber.latitude) : '');
  const [longitude, setLongitude] = useState(houseNumber ? String(houseNumber.longitude) : '');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async (e: SubmitEvent<HTMLFormElement>) => {
    e.preventDefault();
    const parsedNumber = Number(number.trim());
    if (!Number.isInteger(parsedNumber) || parsedNumber < 1) {
      setError(t('houseNumberDialog.invalidNumber'));
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
        streetId,
        number: parsedNumber,
        numberAddition: numberAddition.trim() || null,
        postcode: postcode.trim(),
        ...coordinates,
      });
      onClose();
    } catch (err) {
      setError(errorMessage(err));
      setSubmitting(false);
    }
  };

  return (
    <Stack
      component="form"
      spacing={3}
      className="house-number-dialog__form"
      onSubmit={handleSubmit}
    >
      <ErrorBanner message={error} />
      <Stack direction="row" spacing={2}>
        <TextField
          label={t('fields.number')}
          size="small"
          required
          fullWidth
          autoFocus
          inputMode="numeric"
          value={number}
          onChange={(e) => setNumber(e.target.value)}
        />
        <TextField
          label={t('fields.numberAddition')}
          size="small"
          fullWidth
          placeholder="a"
          value={numberAddition}
          onChange={(e) => setNumberAddition(e.target.value)}
        />
      </Stack>
      <TextField
        label={t('fields.postcode')}
        size="small"
        required
        value={postcode}
        onChange={(e) => setPostcode(e.target.value)}
      />
      <CoordinateFields
        latitude={latitude}
        longitude={longitude}
        onLatitudeChange={setLatitude}
        onLongitudeChange={setLongitude}
      />
      <Stack direction="row" spacing={1} className="house-number-dialog__actions">
        <Button onClick={onClose} disabled={submitting}>
          {t('common.cancel')}
        </Button>
        <Button type="submit" variant="contained" disabled={submitting}>
          {houseNumber ? t('common.save') : t('common.create')}
        </Button>
      </Stack>
    </Stack>
  );
}

/** Creates a house number on the street (`houseNumber` is null) or edits one. */
export default function HouseNumberDialog({
  open,
  streetId,
  houseNumber,
  defaultPostcode = '',
  onClose,
  onSubmit,
}: {
  open: boolean;
  streetId: string;
  houseNumber: HouseNumber | null;
  /** Prefills a new house number, e.g. with the postcode its neighbors use. */
  defaultPostcode?: string;
  onClose: () => void;
  onSubmit: (input: HouseNumberInput) => Promise<void>;
}) {
  const { t } = useTranslation();
  return (
    <Modal
      open={open}
      title={
        houseNumber
          ? t('houseNumberDialog.editTitle', { number: formatHouseNumber(houseNumber) })
          : t('houseNumberDialog.newTitle')
      }
      onClose={onClose}
    >
      <HouseNumberForm
        streetId={streetId}
        houseNumber={houseNumber}
        defaultPostcode={defaultPostcode}
        onClose={onClose}
        onSubmit={onSubmit}
      />
    </Modal>
  );
}
