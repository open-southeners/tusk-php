<?php

namespace Symfony\Component\HttpFoundation;

class Response
{
    /**
     * Set the response content.
     *
     * @param string|null $content
     * @return $this
     */
    public function setContent(?string $content)
    {
        return $this;
    }

    /**
     * Set the response status code.
     *
     * @param int $code
     * @param string $text
     * @return $this
     */
    public function setStatusCode(int $code, string $text = '')
    {
        return $this;
    }

    /**
     * Get the response content.
     *
     * @return string|false
     */
    public function getContent(): string|false
    {
        return '';
    }

    /**
     * Get the response status code.
     *
     * @return int
     */
    public function getStatusCode(): int
    {
        return 200;
    }
}
