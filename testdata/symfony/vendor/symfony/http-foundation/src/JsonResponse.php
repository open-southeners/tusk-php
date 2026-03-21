<?php

namespace Symfony\Component\HttpFoundation;

class JsonResponse extends Response
{
    /**
     * Sets the data to be sent as JSON.
     *
     * @param mixed $data
     * @return $this
     */
    public function setData(mixed $data = [])
    {
        return $this;
    }

    /**
     * Sets the encoding options.
     *
     * @param int $encodingOptions
     * @return $this
     */
    public function setEncodingOptions(int $encodingOptions)
    {
        return $this;
    }
}
