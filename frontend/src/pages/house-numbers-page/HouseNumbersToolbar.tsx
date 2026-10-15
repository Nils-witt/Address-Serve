import { Autocomplete, TextField } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useCities } from '../../hooks/useStreets';
import './HouseNumbersToolbar.scss';

export default function HouseNumbersToolbar({
  city,
  streetName,
  number,
  postcode,
  onCityChange,
  onStreetNameChange,
  onNumberChange,
  onPostcodeChange,
}: {
  city: string;
  streetName: string;
  number: string;
  postcode: string;
  onCityChange: (city: string) => void;
  onStreetNameChange: (streetName: string) => void;
  onNumberChange: (number: string) => void;
  onPostcodeChange: (postcode: string) => void;
}) {
  const { t } = useTranslation();
  const { data: cities } = useCities();

  return (
    <div className="house-numbers-toolbar">
      <TextField
        label={t('streets.searchLabel')}
        size="small"
        value={streetName}
        onChange={(e) => onStreetNameChange(e.target.value)}
      />
      <TextField
        label={t('fields.number')}
        size="small"
        inputMode="numeric"
        className="house-numbers-toolbar__short"
        value={number}
        onChange={(e) => onNumberChange(e.target.value)}
      />
      <TextField
        label={t('fields.postcode')}
        size="small"
        className="house-numbers-toolbar__short"
        value={postcode}
        onChange={(e) => onPostcodeChange(e.target.value)}
      />
      <Autocomplete
        options={cities}
        value={city || null}
        onChange={(_, value) => onCityChange(value ?? '')}
        className="house-numbers-toolbar__select"
        renderInput={(params) => <TextField {...params} label={t('fields.city')} size="small" />}
      />
    </div>
  );
}
