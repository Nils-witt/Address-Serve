import { Autocomplete, Button, TextField } from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import { useTranslation } from 'react-i18next';
import { useCities, useDistricts } from '../../hooks/useStreets';
import './StreetsToolbar.scss';

export default function StreetsToolbar({
  city,
  district,
  name,
  onCityChange,
  onDistrictChange,
  onNameChange,
  onCreate,
}: {
  city: string;
  district: string;
  name: string;
  onCityChange: (city: string) => void;
  onDistrictChange: (district: string) => void;
  onNameChange: (name: string) => void;
  onCreate: () => void;
}) {
  const { t } = useTranslation();
  const { data: cities } = useCities();
  const { data: districts } = useDistricts(city);
  const districtNames = [...new Set(districts.map((d) => d.name))];

  return (
    <div className="streets-toolbar">
      <div className="streets-toolbar__filters">
        <TextField
          label={t('streets.searchLabel')}
          size="small"
          value={name}
          onChange={(e) => onNameChange(e.target.value)}
        />
        <Autocomplete
          options={cities}
          value={city || null}
          onChange={(_, value) => onCityChange(value ?? '')}
          className="streets-toolbar__select"
          renderInput={(params) => <TextField {...params} label={t('fields.city')} size="small" />}
        />
        <Autocomplete
          options={districtNames}
          value={district || null}
          onChange={(_, value) => onDistrictChange(value ?? '')}
          className="streets-toolbar__select"
          renderInput={(params) => (
            <TextField {...params} label={t('fields.district')} size="small" />
          )}
        />
      </div>
      <Button variant="contained" startIcon={<AddIcon />} onClick={onCreate}>
        {t('streets.newStreet')}
      </Button>
    </div>
  );
}
