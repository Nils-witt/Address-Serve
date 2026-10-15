import { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { HouseNumber, HouseNumberInput } from '../api/types';
import ErrorBanner from '../components/ErrorBanner';
import HouseNumberDialog from '../features/house-numbers/HouseNumberDialog';
import { useApi } from '../hooks/useApi';
import { useConfirm } from '../hooks/useConfirm';
import { useDebouncedValue } from '../hooks/useDebouncedValue';
import { useAllHouseNumbers } from '../hooks/useHouseNumbers';
import { useInvalidate } from '../hooks/useInvalidate';
import { useStreets } from '../hooks/useStreets';
import { errorMessage } from '../lib/errors';
import { compareHouseNumbers, formatHouseNumber } from '../lib/format';
import HouseNumbersListTable, {
  type HouseNumberRow,
} from './house-numbers-page/HouseNumbersListTable';
import HouseNumbersToolbar from './house-numbers-page/HouseNumbersToolbar';

/** Orders rows by city, street and then the way numbers appear on a street. */
function compareRows(a: HouseNumberRow, b: HouseNumberRow): number {
  return (
    (a.street?.city ?? '').localeCompare(b.street?.city ?? '') ||
    (a.street?.name ?? '').localeCompare(b.street?.name ?? '') ||
    compareHouseNumbers(a.houseNumber, b.houseNumber)
  );
}

export default function HouseNumbersPage() {
  const { t } = useTranslation();
  const api = useApi();
  const confirm = useConfirm();
  const invalidate = useInvalidate();

  // The filters live in the URL, so a filtered list can be linked and the
  // back button from a street returns to it.
  const [searchParams, setSearchParams] = useSearchParams();
  const city = searchParams.get('city') ?? '';
  const [streetName, setStreetName] = useState(searchParams.get('street') ?? '');
  const [number, setNumber] = useState(searchParams.get('number') ?? '');
  const [postcode, setPostcode] = useState(searchParams.get('postcode') ?? '');
  const debouncedStreetName = useDebouncedValue(streetName.trim());
  const debouncedPostcode = useDebouncedValue(postcode.trim());
  // The server only accepts a whole number; anything else isn't sent.
  const debouncedNumber = useDebouncedValue(/^\d+$/.test(number.trim()) ? number.trim() : '');

  const setFilter = (key: string, value: string) => {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (value) next.set(key, value);
        else next.delete(key);
        return next;
      },
      { replace: true },
    );
  };

  const filter = useMemo(
    () => ({
      city,
      street: debouncedStreetName,
      number: debouncedNumber,
      postcode: debouncedPostcode,
    }),
    [city, debouncedStreetName, debouncedNumber, debouncedPostcode],
  );
  const houseNumbers = useAllHouseNumbers(filter);
  // The streets of the listed house numbers, for their name, district and
  // city. The same city filter keeps this to the streets that can appear.
  const streetFilter = useMemo(() => ({ city }), [city]);
  const streets = useStreets(streetFilter);

  const rows = useMemo(() => {
    const streetsById = new Map(streets.data.map((s) => [s.id, s]));
    return houseNumbers.data
      .map((houseNumber) => ({ houseNumber, street: streetsById.get(houseNumber.streetId) }))
      .sort(compareRows);
  }, [houseNumbers.data, streets.data]);

  // The edited house number stays set while the dialog closes, so its form
  // doesn't change during the exit animation.
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<HouseNumber | null>(null);
  const [error, setError] = useState<string | null>(null);

  const openDialog = (h: HouseNumber) => {
    setEditing(h);
    setDialogOpen(true);
  };

  const onSubmit = async (input: HouseNumberInput) => {
    if (!editing) return;
    await api.updateHouseNumber(editing.id, input);
    await invalidate('houseNumbers');
  };

  const onDelete = async ({ houseNumber, street }: HouseNumberRow) => {
    const ok = await confirm({
      title: t('houseNumbers.deleteTitle'),
      message: t('houseNumbers.deleteMessage', {
        number: formatHouseNumber(houseNumber),
        street: street?.name ?? '',
      }),
      confirmLabel: t('common.delete'),
      destructive: true,
    });
    if (!ok) return;
    setError(null);
    try {
      await api.deleteHouseNumber(houseNumber.id);
      await invalidate('houseNumbers');
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <>
      <HouseNumbersToolbar
        city={city}
        streetName={streetName}
        number={number}
        postcode={postcode}
        onCityChange={(value) => setFilter('city', value)}
        onStreetNameChange={(value) => {
          setStreetName(value);
          setFilter('street', value.trim());
        }}
        onNumberChange={(value) => {
          setNumber(value);
          setFilter('number', value.trim());
        }}
        onPostcodeChange={(value) => {
          setPostcode(value);
          setFilter('postcode', value.trim());
        }}
      />
      <ErrorBanner message={error ?? houseNumbers.error ?? streets.error} />
      <HouseNumbersListTable
        rows={rows}
        loading={houseNumbers.loading || streets.loading}
        onEdit={openDialog}
        onDelete={(row) => void onDelete(row)}
      />
      <HouseNumberDialog
        open={dialogOpen}
        streetId={editing?.streetId ?? ''}
        houseNumber={editing}
        onClose={() => setDialogOpen(false)}
        onSubmit={onSubmit}
      />
    </>
  );
}
